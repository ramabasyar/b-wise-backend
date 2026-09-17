-- ============================================================
-- siakad_staging.sql — schema mirror SIAKAD Binawan (SEVIMA)
-- Lokasi: DB scheduling service (scheduling_db), schema terpisah `siakad_staging`
--
-- PRINSIP DESAIN:
--   1. Staging = raw mirror struktur siakad apa adanya + meta sinkronisasi.
--      TIDAK dinormalisasi untuk domain B-Wise — itu tugas transform (Go, ke public schema).
--   2. Idempotent: semua tabel keyed by natural key siakad (NIM, NIP, Kode ruang,
--      KelasID, dst) → re-scrape/re-import aman via ON CONFLICT DO UPDATE.
--      Baris hilang dari siakai tidak dihapus di staging (mirror snapshot terbaru),
--      tapi terdeteksi oleh transform via anti-join → master di-deactivate (bukan delete).
--   3. Konsumen eksternal (doorlock, sistem lain) TIDAK membaca staging maupun DB
--      secara langsung — selalu lewat API scheduling service (consumer API, published
--      timetable). Staging berisi PII (mahasiswa) → tidak pernah diexpose API publik.
--   4. Tabel ini DIKELOLA OLEH ETL SQL/python — GORM AutoMigrate TIDAK menyentuh
--      schema ini (GORM hanya public schema).
--
-- Diagram alur:
--   [SEVIMA SIAKAD] --scrape(python, auto-relogin)--> [siakad_staging]
--   [workbook xlsx] --etl(python)-------------------> [siakad_staging]
--   [siakad_staging] --transform(Go /api/siakad/transform)--> [master public schema]
--   [master] --solve--> [timetable_versions published] --consumer API+webhook--> [doorlock/dll]
-- ============================================================

CREATE SCHEMA IF NOT EXISTS siakad_staging;

-- ---------- audit sinkronisasi ----------
-- Satu baris per run (scrape ATAU import workbook). stats = jsonb perhitungan baris.
CREATE TABLE IF NOT EXISTS siakad_staging.sync_runs (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source      varchar(20)  NOT NULL,                   -- 'workbook' | 'scrape'
  script      varchar(150),                            -- nama script/endpoint
  status      varchar(20)  NOT NULL DEFAULT 'running', -- running | done | failed
  started_at  timestamptz  NOT NULL DEFAULT now(),
  finished_at timestamptz,
  stats       jsonb        NOT NULL DEFAULT '{}',
  notes       text
);

-- ---------- periode akademik ----------
-- kode = ID internal siakad (contoh '20261' = 2026/2027 Gasal, '20252' = 2025/2026 Genap)
CREATE TABLE IF NOT EXISTS siakad_staging.periode (
  kode        varchar(10)  PRIMARY KEY,
  nama        varchar(100) NOT NULL,
  is_aktif    boolean      NOT NULL DEFAULT false,
  scraped_run uuid,
  scraped_at  timestamptz  NOT NULL DEFAULT now()
);

-- ---------- pegawai (dosen & tenaga kependidikan) ----------
CREATE TABLE IF NOT EXISTS siakad_staging.pegawai (
  nip          varchar(30)  PRIMARY KEY,
  nama         varchar(150) NOT NULL,
  lp           varchar(1),
  nidn         varchar(30),
  nuptk        varchar(50),
  telp         varchar(30),
  email        varchar(150),
  status       varchar(30),
  scraped_run  uuid,
  scraped_at   timestamptz  NOT NULL DEFAULT now()
);

-- ---------- ruang ----------
CREATE TABLE IF NOT EXISTS siakad_staging.ruang (
  kode         varchar(50)  PRIMARY KEY,
  nama         varchar(150),
  unit         varchar(150),
  lokasi       varchar(150),
  kapasitas    integer,
  aktif        boolean,
  scraped_run  uuid,
  scraped_at   timestamptz  NOT NULL DEFAULT now()
);

-- ---------- katalog mata kuliah (semua kurikulum) ----------
-- Kunci alami: (kurikulum, kode) — kode MK bisa sama lintas kurikulum.
CREATE TABLE IF NOT EXISTS siakad_staging.matakuliah (
  kurikulum     varchar(100) NOT NULL,
  kode          varchar(30)  NOT NULL,
  nama          varchar(200) NOT NULL,
  sks_total     numeric(4,1),
  sks_tm        numeric(4,1),
  sks_praktikum numeric(4,1),
  sks_pl        numeric(4,1),
  sks_simulai   numeric(4,1),
  prodi         varchar(150),
  koordinator   varchar(150),
  scraped_run   uuid,
  scraped_at    timestamptz  NOT NULL DEFAULT now(),
  PRIMARY KEY (kurikulum, kode)
);

-- ---------- kelas / offering ----------
-- kelas_id = data-id internal siakad (stabil) — kunci join ke pengajar & peserta.
CREATE TABLE IF NOT EXISTS siakad_staging.kelas (
  kelas_id      varchar(30)  PRIMARY KEY,
  periode       varchar(10)  NOT NULL,               -- FK siakad_staging.periode.kode
  kurikulum     varchar(100),
  mk_kode       varchar(30),
  mk_nama       varchar(200),
  prodi         varchar(150),
  nama_kelas    varchar(100),                        -- label rombel: 'A', 'TI-3A', dst
  pengajar_raw  text,                                -- gabungan dosen apa adanya
  jadwal_raw    text,                                -- jadwal mingguan siakad (ground truth)
  kapasitas     integer,
  peserta       integer,                             -- hitungan saat scrape
  jumlah_dosen  integer,
  team_teaching boolean,
  pola          varchar(60),                         -- Single | TT Sejajar | Split Periode UTS | ...
  scraped_run   uuid,
  scraped_at    timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_kelas_periode ON siakad_staging.kelas (periode);
CREATE INDEX IF NOT EXISTS idx_kelas_mk ON siakad_staging.kelas (mk_kode);

-- ---------- pengampu per kelas (presisi, hasil crawl data_pengajar/detail) ----------
-- Satu baris per (kelas, dosen) — termasuk porsi SKS & pola sesi.
CREATE TABLE IF NOT EXISTS siakad_staging.pengajar (
  kelas_id     varchar(30)  NOT NULL,                -- FK siakad_staging.kelas.kelas_id
  nip          varchar(30)  NOT NULL,
  nama         varchar(150),
  porsi_sks    numeric(4,1),
  sesi_periode varchar(100),                         -- 'Seluruh Sesi (1-16)' | 'Sebelum UTS (1-8)' | ...
  jadwal_raw   text,
  status       varchar(30),
  urutan       integer,
  scraped_run  uuid,
  scraped_at   timestamptz  NOT NULL DEFAULT now(),
  PRIMARY KEY (kelas_id, nip)
);

-- ---------- peserta kelas (enrollment / KRS) ----------
CREATE TABLE IF NOT EXISTS siakad_staging.peserta_kelas (
  kelas_id   varchar(30) NOT NULL,
  nim        varchar(20) NOT NULL,
  nama_mhs   varchar(150),
  prodi      varchar(150),
  angkatan   varchar(10),
  status_krs varchar(50),                            -- Disetujui | Tidak Disetujui | ...
  scraped_run uuid,
  scraped_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (kelas_id, nim)
);
CREATE INDEX IF NOT EXISTS idx_peserta_nim ON siakad_staging.peserta_kelas (nim);

-- ---------- mahasiswa (populasi lengkap — PII, tidak diexpose API) ----------
CREATE TABLE IF NOT EXISTS siakad_staging.mahasiswa (
  nim               varchar(20)  PRIMARY KEY,
  nama              varchar(150) NOT NULL,
  nik               varchar(30),
  jk                varchar(10),
  agama             varchar(20),
  prodi             varchar(150),
  kelas             varchar(50),                     -- kelas perkuliahan
  konsentrasi       varchar(100),
  tempat_lahir      varchar(100),
  tanggal_lahir     varchar(50),
  alamat            text,
  rt                varchar(5),
  rw                varchar(5),
  dusun             varchar(100),
  desa              varchar(100),
  kecamatan         varchar(100),
  kota              varchar(100),
  provinsi          varchar(100),
  kode_pos          varchar(10),
  hp                varchar(30),
  email_kampus      varchar(150),
  email_pribadi     varchar(150),
  jenis_pendaftaran varchar(50),
  asal_instansi     varchar(150),
  nisn              varchar(30),
  bk                varchar(100),                    -- berkebutuhan khusus
  tgl_lahir_ijazah  varchar(50),
  nama_ayah         varchar(150),
  pekerjaan_ayah    varchar(100),
  penghasilan_ayah  varchar(100),
  bk_ayah           varchar(100),
  nama_ibu          varchar(150),
  pekerjaan_ibu     varchar(100),
  penghasilan_ibu   varchar(100),
  bk_ibu            varchar(100),
  scraped_run       uuid,
  scraped_at        timestamptz  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_mhs_prodi ON siakad_staging.mahasiswa (prodi);
CREATE INDEX IF NOT EXISTS idx_mhs_kelas ON siakad_staging.mahasiswa (kelas);

-- ---------- snapshot mhs aktif per periode ----------
-- Sumber: sheet mhs aktif per semester (populasi kecil — semester baru belum roll-out).
CREATE TABLE IF NOT EXISTS siakad_staging.mahasiswa_aktif (
  nim         varchar(20) NOT NULL,
  periode     varchar(10) NOT NULL,
  nama        varchar(150),
  jk          varchar(10),
  prodi       varchar(150),
  kelas       varchar(50),
  email       varchar(150),
  scraped_run uuid,
  scraped_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (nim, periode)
);

-- ---------- sesi jadwal (ground truth siakad 2026G — validasi hasil solve) ----------
-- Distinct (kelas, hari, jam, jenis, ruang, dosen) dari rep_jadwalkuliah.
-- Reload penuh per periode tiap run (tidak ada natural key stabil).
CREATE TABLE IF NOT EXISTS siakad_staging.sesi_jadwal (
  id           serial PRIMARY KEY,
  periode      varchar(10)  NOT NULL,
  kuliah_kelas varchar(200),
  hari         varchar(10),
  jam_mulai    varchar(5),
  jam_selesai  varchar(5),
  jenis        varchar(30),                          -- Kuliah | Praktikum | UTS | UAS | Klinik/Lapang
  metode       varchar(30),
  ruang        varchar(50),
  dosen        text,
  scraped_run  uuid,
  scraped_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sesi_periode ON siakad_staging.sesi_jadwal (periode);
CREATE INDEX IF NOT EXISTS idx_sesi_ruang  ON siakad_staging.sesi_jadwal (ruang);
