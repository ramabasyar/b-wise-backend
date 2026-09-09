# ROADMAP — Human Capital Service (B-Wise)

> **Status:** Living document — update tiap kali fase dimulai/selesai.
> **Terakhir diperbarui:** 2026-08-18
> **Owner:** Mas Rama (arsitektur & prioritas) · Em (implementasi & dokumentasi)
> **Cara pakai:** Tiap item punya checkbox. Tandai saat mulai (`~` di teks) dan selesai (`[x]`). Urutan fase = urutan rekomendasi, tapi **bisa diubah** sesuai kebutuhan kampus.

---

## 1. Visi

Service Human Capital = **HRM modern pengganti Talenta/Frappe HR** untuk Universitas Binawan — dibangun **bertahap**, setiap fase harus langsung bisa dipakai (usable increment), bukan big-bang.

### Prinsip Arsitektur

1. **Satu service, module boundary jelas.** Domain internal terpisah per modul (`internal/...` per aggregate: employee, leave, attendance, payroll). Split jadi service terpisah HANYA kalau payroll terbukti terlalu berat (beban hitung / isolated scaling) — jangan premature split.
2. **Data historis, bukan snapshot.** Perubahan penting (jabatan, gaji, status) dicatat sebagai riwayat ber-tanggal (effective-dated), bukan overwrite field.
3. **Aturan berubah = tabel parameter.** Bracket PPh 21, upah minimum, batas BPJS, hari kerja — semua di tabel parameter berperiode, JANGAN hardcode.
4. **Permission-first.** Tiap modul baru wajib: daftar scope `{human-capital}:{resource}:{action}` → seed ke permission-service (self-register manifest kalau sudah di-wire, atau manual) + menu item + tombol UI ter-filter.
5. **Audit trail semua mutasi** penting (grant, perubahan gaji, approval) — best-effort write, gak blocking.
6. **Clean Architecture konsisten** dengan service lain: handler → DTO → service → repository, Response DTO tanpa field internal.

### Non-Goals (sengaja TIDAK dibangun service ini)

- Penggajian lewat pihak ketiga (outsourcing payroll) — kalau dipilih jalur itu, fase payroll dibatasi "payroll record & reporting", bukan engine hitung.
- Akuntansi/finansial (jurnal, COA) — wilayah service finansial lain (kalau nanti ada).

---

## 2. Kondisi Saat Ini (Fase 1 — SELESAI ✅ + REBUILD Frappe HR-aligned 18 Aug)

> **REBUILD 18 Aug 2026:** Core HR dibangun ulang dari nol benchmark **Frappe HR/HRMS** (lihat `HC_DESIGN.md`):
> Employee master kaya (identitas Indonesia: NIK/NPWP/BPJS, employment, exit), Department **tree**, Designation, Employment Type (seed PNS/PPPK/PKWT/dst), Grade, Branch, **EmployeeMovement + property-history detail** (promosi/mutasi/separation transaksional), onboarding wizard (SSO+employee+role), **self-register manifest** (`perm.manifest.yaml` → permission-service auto-sync saat boot). NIP auto-generate BIN-YYYYMM-####. Scope lama `positions.*` deprecated → `designations.*`.

| Modul | Status | Keterangan |
|---|---|---|
| Employee master dasar | ✅ | CRUD, pagination, status aktif/nonaktif |
| Department | ✅ | CRUD + code unik |
| Position | ✅ | CRUD + level |
| Onboarding wizard | ✅ | 3 langkah: akun SSO → profil → role/akses (integrasi permission-service) |
| Auth & permission | ✅ | JWKS + middleware v2 + scope `employees/departments/positions.*` |
| UI portal | ✅ | Semua halaman Figma-design di `next/b-wise` |

**Utas teknis yang belum rapi (boleh dibereskan sambil jalan):**
- [ ] `joined_date` masih bebas — belum validasi terhadap kontrak/onboard
- [ ] Belum ada soft-delete konsisten di semua entity (employee ada, dept/posisi cek ulang)
- [ ] Endpoint list department/position belum pagination (data kecil, tidak urgent)

---

## 3. Overview Fase

| Fase | Modul | Nilai | Estimasi | Status |
|---|---|---|---|---|
| 1 | Core HR dasar | Fondasi | M | ✅ DONE |
| 2 | **Leave + Attendance dasar + ESS** | Dipakai harian semua pegawai | L | ⬜ |
| 3 | Core HR lanjut (movement, grade, dokumen, jenis kepegawaian) | Melengkapi fondasi pra-payroll | M | ⬜ |
| 4 | **Payroll + BPJS + PPh 21 + THR** | Killing feature pengganti Talenta | XL | ⬜ |
| 5 | Performance (link IKU) + L&D | Menyambung ekosistem B-Wise | L | ⬜ |
| 6 | Recruitment + Analytics lanjut | Opsional, saat butuh | M | ⬜ |

> Estimasi relatif: S (hari) · M (1–2 minggu) · L (2–4 minggu) · XL (per bulan, pecah sub-fase) — asumsi kerja paruh waktu, tanpa deadline keras.

---

## 4. FASE 2 — Leave, Attendance Dasar & ESS

**Tujuan:** pegawai bisa clock-in/out + ajukan cuti + lihat saldo; atasan approve — nilai harian langsung terasa.

### 4.1 Leave Management
- [ ] Entity `leave_types` (tahunan, sakit, melahirkan, menikah, ibadah, cuti bersama, tanpa gaji) + kuota default per jenis kepegawaian
- [ ] Entity `leave_balances` (per pegawai per tahun, carry-over max N hari — parameter)
- [ ] Entity `leave_requests` + status workflow: `draft → submitted → approved/rejected → cancelled`
- [ ] Approval sederhana 1 level (atasan langsung dari relasi position/reporting — butuh field `reports_to` di employee, lihat catatan)
- [ ] Kalender kerja & hari libur nasional (tabel `holidays`, bisa import)
- [ ] Endpoint: pengajuan, approval (approve/reject + alasan), saldo, riwayat
- [ ] UI: halaman "Cuti Saya" (ESS) + halaman "Approval Cuti" (approver) + admin leave-type config

### 4.2 Attendance Dasar
- [ ] Entity `attendance_records` (check-in/out, tanggal, metode)
- [ ] Check-in/out via **web portal** (ESS) dulu — jam server, deteksi telat via jam kerja default
- [ ] Jam kerja default + toleransi (parameter per jenis hari)
- [ ] Laporan kehadiran sederhana per pegawai/periode (admin)
- [ ] **Defer ke sub-fase berikutnya:** shift scheduling, geo-fencing, integrasi mesin fingerprint (butuh riset vendor mesin kampus), log jam mengajar dosen

### 4.3 ESS Dasar (Employee Self Service)
- [ ] Halaman "Profil Saya" kaya: data pribadi + ajukan perubahan data (light approval)
- [ ] Ringkasan: saldo cuti, riwayat kehadiran 30 hari
- [ ] Dasbor admin: siapa yang telat/absen hari ini

### Kriteria Selesai (DoD Fase 2)
- [ ] Scope baru ter-seed: `leaves.*`, `attendance.*`, `ess.*` + menu portal
- [ ] E2E: pegawai ajukan cuti → atasan approve → saldo berkurang
- [ ] E2E: check-in/out web tercatat + muncul di laporan

### Catatan Desain
- `reports_to`: tambahkan kolom `reporting_to` (FK position/employee) saat fase ini — approval butuh relasi atasan. ⚠️ ini perubahan schema employee.
- Workflow approval: mulai 1-level sederhana (state machine di service layer, tabel request + status). Jangan import workflow engine penuh dulu.

---

## 5. FASE 3 — Core HR Lanjut (pra-Payroll)

**Tujuan:** data kepegawaian lengkap & benar secara historis — prasyarat payroll akurat.

- [ ] **Jenis kepegawaian** (`employment_types`: PNS, PPPK, tetap yayasan, PKWT, dosen luar biasa) + relasi ke employee (historis)
- [ ] **Job grade / level / pangkat** (matrix grade × jabatan; golongan utk PNS)
- [ ] **Employee movement history** (`employee_movements`: mutasi, promosi, demotion, perubahan status, resign — dengan tanggal efektif + dokumen pendukung)
- [ ] **Dokumen pegawai** (`employee_documents`: KTP, NPWP, kontrak, ijazah, sertifikat — upload ke MinIO/S3 kalau infrastruktur siap, sementara bisa metadata + URL)
- [ ] **Offboarding**: proses resign/pensiun + checklist clearance (IT, perpustakaan, keuangan) sederhana
- [ ] Kontrak berbasis periode (PKWT auto-expire reminder)
- [ ] UI: timeline riwayat di profil pegawai, form movement, upload dokumen, halaman offboarding

### Kriteria Selesai (DoD Fase 3)
- [ ] Demo: promote pegawai → jabatan & grade berubah dengan riwayat tercatat, gaji belum (payroll fase 4)
- [ ] Semua perubahan status melalui movement (tidak ada overwrite langsung tanpa jejak)

---

## 6. FASE 4 — Payroll, BPJS, PPh 21 & THR

**Tujuan:** pengganti penuh Talenta untuk penggajian kampus. **Pecah jadi sub-fase, jangan sekali jalan.**

### 4a. Struktur Kompensasi
- [ ] Entity komponen gaji (`pay_components`: gaji pokok, tunjangan tetap/tidak tetap, potongan) — per pegawai, **effective-dated**
- [ ] Template gaji per jenis kepegawaian/grade
- [ ] Riwayat perubahan gaji (movement `salary_change`)

### 4b. Parameter Regulasi (tabel, bukan hardcode)
- [ ] `pph21_brackets` (termasuk **TER** — tarif efektif bulanan/DEP)
- [ ] `bpjs_parameters`: batas atas/bawah, persentase Kesehatan + Ketenagakerjaan (JHT/JP/JKK/JKM), iuran Pensiun
- [ ] `thr_rules` (prorate <12 bulan, proporsional)
- [ ] UMK/UMR per tahun + status PTKP (kawin/tanggungan)

### 4c. Payroll Run Engine
- [ ] `payroll_periods` (bulanan) + `payroll_runs` (draft → finalized)
- [ ] Kalkulasi per pegawai: komponen + BPJS + PPh 21 (metode: gross/ gross-up/ TER — pilih default)
- [ ] Lembur masuk hitungan (dari attendance fase 2 — tarif via parameter)
- [ ] Slip gaji (PDF) + distribusi (email/WA opsional)
- [ ] Rekapan: lapor 1721 (format CSV/XLSX expor), laporan BPJS
- [ ] Audit: siapa finalize, kapan, snapshot parameter saat run (reproducible)

### 4d. THR Run
- [ ] Perhitungan THR per periode (prorate + parameter)
- [ ] Slip THR + rekap

### Kriteria Selesai (DoD Fase 4)
- [ ] Payroll run bulanan penuh dengan hasil **cocok dengan hitungan manual/acuan** (test case pegawai PNS + PKWT + PPPK)
- [ ] Parameter tahunan bisa di-roll tanpa migrasi kode
- [ ] Slip gaji download ESS

> ⚠️ **Keputusan yang perlu diambil sebelum fase ini:** apakah kampus ingin engine hitung sendiri (penuh) atau cukup "recording + reporting" sambil payroll tetap di vendor. Ini mengubah scope 4c total.

---

## 7. FASE 5 — Performance & L&D

**Tujuan:** menyambungkan kinerja individu dengan IKU institusi (ekosistem B-Wise).

- [ ] **Performance:**
  - [ ] Target/karyawan per periode (turunan KPI unit ← terhubung service IKU via API)
  - [ ] Penilaian periodik (skala + kategori), review atasan, SKP utk PNS (format khusus)
  - [ ] Laporan kinerja per unit → feed dashboard IKU
- [ ] **L&D:**
  - [ ] Katalog pelatihan/seminar + pendaftaran internal
  - [ ] Riwayat pelatihan & sertifikasi (termasuk Serdos utk dosen)
  - [ ] Rekam jejak publikasi & pengabdian dosen (scope: metadata saja, integrasi SINTA opsional)

---

## 8. FASE 6 — Recruitment & Analytics Lanjutan

- [ ] **Recruitment:** lowongan → pelamar (form publik) → screening → interview schedule → offer → auto-onboard (nyambung ke wizard onboarding existing!) 
- [ ] **Analytics:** headcount & demografi, turnover rate, absenteeism trend, biaya gaji per unit/trend, komposisi jenis kepegawaian
- [ ] Export & scheduled report (bulanan via WA/email — pola seperti standup office_assistent)

---

## 9. Cross-Cutting (berlaku semua fase)

| Kepentingan | Standar |
|---|---|
| Permission scope | `{human-capital}:{resource}:{action}` → seed permission-service + menu item + filter UI |
| Audit | Semua mutasi penting → tabel audit (perm-service global atau lokal per service — konsisten pilih satu) |
| Testing | Tiap fase: unit test service layer + E2E happy path via API (minimal), UI E2E utk flow utama |
| Dokumentasi | Endpoint baru masuk `b-wise/API_DOCUMENTATION.md`; update file ini tiap milestone |
| Migrasi DB | AutoMigrate + seed idempoten; perubahan breaking → migrasi terdokumentasi |
| UI | Semua halaman baru wajib pakai component library Binawan (`next/b-wise/src/components/brand`) |

---

## 10. Open Questions (perlu keputusan Mas Rama)

1. **Payroll engine vs recording?** (menentukan scope Fase 4 total) 
2. **Mesin fingerprint kampus** — vendor/tipe? (menentukan effort sub-fase attendance lanjutan; kalau tidak ada, geo-fencing mobile app jadi opsi)
3. **Approval multi-level?** Cuti/lembur 1 level cukup, atau rektorat butuh chain approval (dekan → HR → rektor)?
4. **Storage dokumen** — pakai MinIO (sudah ada pengalaman di office_assistent) atau filesystem dulu?
5. **Data pegawai lama** — apakah perlu import bulk dari Excel (migrasi dari sistem lama/Talenta)? Kalau ya, tool import perlu masuk fase mana pun sebelum payroll live.
6. **Multi-kampus/unit usaha?** Kalau Binawan punya unit di luar kampus (RS? sekolah?), struktur org perlu `org_unit` tree sejak awal — dampak ke schema fase 3.

---

## 11. Riwayat Perubahan

| Tanggal | Perubahan |
|---|---|
| 2026-08-18 | Dokumen dibuat — hasil diskusi scope HRM (benchmark Talenta/Frappe HR/Odoo). Fase 1 ditandai selesai. |
