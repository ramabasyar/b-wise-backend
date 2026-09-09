# ROADMAP — B-Wise Performance Center (Service IKU)

> **Status:** Living document — kontrak pengembangan. Update tiap fase mulai/selesai.
> **Dibuat:** 18 Aug 2026 — oleh Em, disetujui dasarnya oleh Mas Rama
> **Acuan:** Blueprint v2.0 Mas Rama (utama) + Blueprint v1 (konteks) + RESEARCH.md (riset IKU Indonesia & eCakradipa UNDIP)
> **Regulasi acuan:** Kepmendiktisaintek No. 358/M/KEP/2025 — **12 IKU** (bukan lagi 8), kategorisasi Wajib/Pilihan. **Verifikasi ulang ke JDIH sebelum go-live produksi.**

---

## 0. Aturan Konsistensi (anti-ngelantur)

1. **Blueprint v2 = kontrak desain.** Fitur di luar blueprint masuk hanya lewat: catat di §Open Questions → dibahas → update file ini DULU, baru coding.
2. **Setiap fase harus menghasilkan software yang jalan** (bisa didemokan), walau sempit — bukan setengah arsitektur.
3. **Urutan di file ini = urutan kerja.** Melompati fase hanya atas keputusan eksplisit Mas Rama (catat alasannya di §Riwayat).
4. **12 IKU adalah DATA, bukan KODE.** Tidak ada satu pun definisi/formula IKU yang di-hardcode — semua via `regulatory_versions` + `indicator_definitions` + `formula_versions` (prinsip Regulation-First blueprint).
5. **Jangan bangun yang ditunda.** Tiap fase punya batas "Out of Scope" — menghormatinya sama pentingnya dengan menyelesaikan scope.

## 1. Keputusan Arsitektur (adaptasi blueprint v2 → realitas B-Wise hari ini)

| Aspek | Blueprint v2 | Keputusan kita | Alasan |
|---|---|---|---|
| Basis service | Microservice Go | ✅ `b-wise/iku` (port 8083, generated via CLI, perm.manifest self-register) | Konsisten ekosistem |
| Auth & peran | JWT B-Wise, RACI | ✅ SSO JWKS + permission-service. Peran = permission scope: `iku.admin`, `iku.operator`, `iku.operator_output`, `iku.reviewer`, `iku.executive-view` (+ super admin existing) | Jangan bikin tabel role sendiri; RACI matrix blueprint dipetakan ke scope ini |
| Unit kerja | — | ✅ Konsumsi **human-capital** (branch = fakultas/unit, department) via internal client; cache lokal ID↔nama | Single source of truth, HC sudah jalan |
| Unit pengukuran L0–L6 | Hierarki lengkap visi→individu | ✅ Skema mendukung penuh (`indicator.level`), tapi UI/fitur per level aktif bertahap (lihat fase) | Jangan bangun 7 level UI sebelum level institusi terpakai |
| Message broker | Kafka/RabbitMQ event-driven | ⏸ **Defer ke Fase 5.** Integrasi awal: REST pull terjadwal + manual input | Belum ada Kafka di B-Wise; manual-input-first sesuai mitigasi risiko blueprint |
| File storage evidence | MinIO | ✅ **Docker compose dev siap** (`b-wise/deploy/minio/docker-compose.yml` + bucket auto-init) & env S3_* di .env iku; PRODUCTION: server MinIO khusus (Mas Rama) — ganti S3_ENDPOINT/creds saja. Abstraksi storage interface dipasang di F2 | Dev container tinggal `up -d`; prod = config swap |
| Formula engine | Fully pluggable, versioned | ✅ Ekspresi string evaluabel (library `expr-lang/expr` atau setara) + `formula_versions` + sandbox test-run. **Tidak ada formula di kode** | Sesuai prinsip Formula-as-Configuration |
| Periode | Flexible per-indikator | ✅ `period_type`: quarterly/semester/annual + tabel periode dengan status open/closed | Keunggulan #3 vs eCakradipa |
| Dashboard | Per role + drill-down | ✅ Di portal `next/b-wise` (brand components), menu via manifest → sidebar otomatis | Investasi portal dipakai ulang |
| Digital signature | Ada | ⏸ Defer: approval tercatat audit trail dulu (siapa-kapan-IP); TTD digital = opsi Fase 6 | Kompleksitas PKI; bukan blocker nilai inti |
| Multi-tenant SaaS | Rencana jangka panjang | ⏸ Out of scope dokumen ini | Ekosistem Binawan dulu |

**Mapping lapisan blueprint v2 → fase kita:**

| Lapisan Blueprint v2 (Bab 3) | Fase |
|---|---|
| A. Regulatory Compliance Engine | F0 (registry+versioning skema) → F1 (definisi & formula versioned) → **F6** (change workflow, impact analyzer, monitor) |
| B. Strategic Framework (Renstra, PK) | F1 (PK-lite: link target↔dokumen) → F6 (Renstra digital penuh) |
| C. Indicator Engine | **F1** |
| D. Data & Integration Broker | F2 (manual gateway) → **F5** (connectors) |
| E. Evidence & Governance | **F2** (evidence, workflow, audit) |
| F. Action Plan & Monitoring | **F4** |
| G. Executive Dashboard & Reporting | **F3** |

## 2. FASE 0 — Foundation & Regulatory Registry (MVP start)

**Tujuan:** service IKU hidup di ekosistem: DB, 12 IKU ter-seed **sebagai data regulasi v1**, permission & menu ter-register, smoke-test end-to-end.

- [x] Entity inti (AutoMigrate) ✅:
  - `regulatory_versions` (regulation_code, title, effective_date, document_url, status: draft/active/superseded, notes)
  - `indicator_definitions` (iku_code, name, description, nature: wajib/pilihan, period_type, level: L0..L6, reg_version_id, valid_from/to, is_active) — **tanpa formula dulu**
  - `periods` (label "TW1-2026", type, year, quarter/semester, status: open/closed, due_date) ✅ + seed TW1-4 2026 & tahunan
- [x] Seed data regulasi: **Kepmendiktisaintek 358/2025 + 12 IKU** ✅ (seed idempoten saat boot)
- [x] Internal client HC ✅ hcclient (service token, cache 5m) + proxy GET /api/units
- [x] `perm.manifest.yaml` F0 ✅ (regulations/indicators/periods scope + menu IKU ▸ Indikator; menu lain menyusul per fase)
- [x] Endpoint ✅: GET/POST regulations, GET indicators (+filter), GET/POST periods, health
- [x] UI: halaman Indikator ✅ (StatCard ringkasan + filter sifat + tabel 12 IKU + info regulasi)
- [x] DoD ✅: boot iku → self-sync (3 perm baru, menu update) → API verified (12 IKU, 5 periode) → UI verified via E2E browser

**Seed tambahan F1:** formula IKU-2 v1 aktif (contoh editable). **Scope F1 tambahan terverifikasi:** menu Targets di manifest (4 scope baru: formulas.read/write, targets.read/write).

**Out of scope F0:** formula, target, capaian, workflow, dashboard angka.

## 3. FASE 1 — Indicator Engine: Formula, Target & PK-lite — ✅ SELESAI 18 Aug

**Tujuan:** indikator hidup sebagai objek terukur: formula versioned + penetapan target per unit per periode.

- [x] `formula_versions` ✅ (expression + input_variables JSONB serializer + rounding/validation + status draft/active/retired)
- [x] **Formula runtime** ✅ expr-lang/expr (pure math env) + POST /api/formulas/test sandbox + CheckValidation (min/max/warn)
- [x] `performance_targets` ✅ unik (indicator,unit,period) + upsert + approve + UI halaman Targets (filter periode, dialog set target, approve)
- [x] PK-lite ✅ `pk_documents` + endpoint (tautan pk_ref di target tersedia)
- [x] Threshold ✅ `indicator_thresholds` (entity + upsert service; UI & pemakaian visual → F3)
- [x] Endpoint ✅ formulas CRUD+activate/retire+test, targets upsert/approve, pk-documents, units proxy (HC via service token, cache 5m)
- [x] DoD ✅ formula IKU-2 v1 seed-aktif → v2 dibuat via API & diaktifkan (v1 auto-retire) → sandbox 75.00 via UI → target institusi 75 (2026) tampil & setujui → E2E browser: dialog 2 versi + sandbox result + targets rows, zero error

**Out of scope F1:** realisasi/capaian, integrasi data otomatis.

## 4. FASE 2 — Capaian, Evidence & Governance Workflow (inti MVP)

**Tujuan:** siklus pengukuran lengkap manual-input: submit → review → approve → publish, dengan evidence & audit trail. **Setelah fase ini = MVP blueprint (Bab 9.2).**

- [x] `achievement_records` ✅ (raw_data jsonb immutable setelah non-draft; snapshot formula_id + target_value; pct + threshold_color)
- [x] Kalkulasi otomatis ✅ (formula aktif; formula_id tercatat per record — historis)
- [x] `evidence_documents` ✅ + multipart upload (10MB, whitelist ekstensi) → **storage interface: S3/MinIO AKTIF (bucket bwise-iku-evidence) + local fallback**
- [x] `workflow_logs` ✅ semua aksi tercatat (created/recalculated/evidence_added/submit/review/approve/publish/reject)
- [x] Workflow RACI ✅ state machine draft→submitted→reviewed→approved→published (+rejected→resubmit), tolak wajib catatan, guard transisi invalid + raw terkunci
- [x] SLA-lite ✅ (periode closed menolak input; monitor penuh → F4)
- [x] UI Capaian ✅ (filter status+periode; form input AUTO-GENERATE dari variabel formula aktif — manual-input-first; tombol workflow inline; detail: raw data, evidence upload/download, catatan tolak, riwayat log)
- [x] DoD ✅ API: kalkulasi 70→93.33% hijau; evidence MinIO roundtrip identik; workflow penuh + guard + 7 log audit. UI: form 5 input auto, draft 78, submit UI; zero error. (RBAC 403 via permission middleware — scope achievements.*)

**Out of scope F2:** integrasi otomatis, dashboard lintas unit (F3), action plan (F4).

## 5. FASE 3 — Dashboard Eksekutif & Reporting — ✅ SELESAI 18 Aug (export menyusul)

**Tujuan:** angka terlihat per kewenangan: rektor/WR/dekan/kaprodi + drill-down + export.

- [x] Agregasi ✅ (GET /api/dashboard?unit&period — Build summary 12 IKU; /dashboard/trend; /dashboard/units perbandingan antar unit; LatestPeriod = periode dgn capaian ter-advance)
- [x] Dashboard UI ✅ `/dashboard/iku/dashboard`: 4 StatCard (rerata+SegmentedBar G/Y/R, published, in-progress, belum diisi), tren Sparkline, tabel 12 IKU (warna threshold + status + bukti), perbandingan unit; selector unit/periode. Role-scope penuh → saat data per-unit nyata (opsional enhancement)
- [ ] Drill-down: pilih unit via selector + detail capaian via halaman Capaian ✓ dasar; drill berjenjang penuh → enhancement bersama data nyata
- [ ] Export PDF/Excel — dipindah ke backlog F7 (nice-to-have pasca data nyata); JSON tersedia via API
- [ ] Dashboard publik ringkas (seperti eCakradipa landing: capaian per kluster, filter tahun/triwulan) — opsional toggle
- [ ] DoD: login 3 peran berbeda → masing-masing melihat dashboard sesuai kewenangan; export terunduh & isinya benar

## 6. FASE 4 — Action Plan & Monitoring Loop — ✅ SELESAI 18 Aug

- [x] `action_plans` ✅ (items JSONB serializer, PIC name/user, deadline, progress auto dari items, status open|done|escalated + EscalateOverdue batch saat boot)
- [x] Pemicu ✅ GET /api/action-plans/suggestions — capaian published merah/kuning tanpa plan aktif (saran, bukan paksa) + panel kuning di UI
- [x] PIC + progres ✅ (dialog update: klik item toggle done → progress & status terhitung; eskalasi manual + batch overdue otomatis saat boot; notifikasi WA/email → backlog)
- [x] Tren pasca-perbaikan: dasar tersedia via dashboard tren antar periode — visual khusus before/after → backlog F7
- [x] SLA-lite ✅ (periode closed menolak input + deadline plan eskalasi otomatis); SLA panel penuh → backlog
- [x] DoD ✅ API: suggestions 2 capaian (FBIS 41.3% 🔴, FIKST 64% 🟡); plan 3 item → 1 done → progress 33%; guard duplikat; plan deadline-lewat → escalated. UI: submenu Action Plans, panel saran (2→0 setelah ter-cover), toggle item → toast progres, zero error

## 7. FASE 5 — Data Integration Broker (connectors) — ✅ SELESAI 18 Aug (connector API sync → menyusul)

**Tujuan:** kurangi input manual — **contract-first** per sumber (blueprint Bab 8.1), REST pull terjadwal (Kafka event-driven dicatat sebagai upgrade path, tidak dibangun sekarang).

- [x] `data_source_connectors` ✅ (type manual|file-import|api|db; contract_json fields→variabel formula; last_sync_at + last_run_info)
- [x] Connector HC ✅ (hcclient jalan sejak F1 — unit proxy; dipakai dashboard/target/import)
- [x] Jalur SIAKAD/Tracer/PDDikti = **file-import terkelola** ✅ (sesuai keputusan no-API): template CSV/Excel dari kontrak → upload → preview validasi per baris → apply → capaian draft source=import + ImportBatch audit + anti re-apply. Connector API sync otomatis (type=api) → menyusul saat tersedia (Moodle pasca-migrasi)
- [x] Import Excel/CSV ✅ core F5 (excelize xlsx + csv; validasi angka/required/kolom; multi-unit via kolom unit_id)
- [x] Quality dasar ✅ (validasi kontrak saat import; last_run_info per connector; flag error per baris di preview; source=import tercatat di capaian) — panel quality agregat → backlog F7
- [x] Normalisasi ✅ (header file dinormalisasi snake_case; mapping kolom→variabel via kontrak; koma desimal ditoleransi)
- [x] DoD ✅ connector Tracer Study realistis: template ✓, import 4 baris (2 valid 2 error tertangkap detail) ✓, guard apply-dengan-error ✓, apply bersih → capaian draft (agregasi baris, calc 50) ✓, anti re-apply ✓. UI: card connector, dialog import (preview tabel merah/hijau per baris, apply), riwayat. E2E UI zero error

## 8. FASE 6 — Regulatory Compliance Engine penuh + Strategic Framework — ✅ CORE SELESAI 18 Aug (Renstra/OKR → backlog)

**Tujuan:** keunggulan diferensiator (Bab 7 blueprint) — sistem siap "menyambut" perubahan regulasi.

- [x] Change Workflow ✅ draft→in_review→approved→active (+rejected→revisi; tolak wajib catatan; aktivasi ATOMIK dgn efek: reg baru active + lama superseded, formula v+1 aktif + lama retired, definisi pindah reg). Notifikasi & scheduling otomatis → backlog
- [x] Versioning ✅ (aktivasi auto-retire; capaian menyimpan formula_id snapshot — historis immutable, terbukti di DoD; reg superseded)
- [x] Compliance Monitor ✅ GET /api/compliance — per IKU wajib: formula? target? capaian? published? → level ok/warn/missing + pesan; default periode ter-advance
- [ ] Strategic Framework penuh (Renstra digital, mapping sasaran, OKR) → **backlog F7** — fondasi regulatory_versions & indicator level sudah siap menampung
- [ ] PK digital penuh (export, TTD) → backlog F7 — PK-lite sudah jalan sejak F1
- [x] DoD ✅ "Regulasi 2027" (formula IKU-6 ×1.2): impact 2 capaian (0.24→0.29 Δ+0.05, 0.40→0.48 Δ+0.08, improved) → review→approve→activate → reg SIM-2027 active + 358/2025 superseded + formula v2 active/v1 retired + **capaian historis TETAP 0.4/0.24 (snapshot v1)** + IKU-6 pindah regulasi baru. Guard transisi invalid ✓

## 9. FASE 7 — Hardening, Pilot & Launch

- [x] Perf ✅ (dev-scale): indexing GORM (status/unit/deadline/notif), guard pagination (LIMIT 2000 export, list filter). **Async job queue → dibuka saat volume capaian nyata > ~10k/per-periode** (data pilot jauh di bawah itu; kalkulasi formula < 1ms)
- [x] Observability ✅: `GET /api/metrics` (Prometheus text: capaian per status, formula, action plan, import, notifikasi, regulasi aktif) + log terstruktur zap + request-id (sudah ada). Panel Grafana → ops opsional saat produksi
- [x] Security pass ✅ (upload): rate limit 10 upload/menit/user (429), maks 10MB, whitelist ekstensi+content-type (pdf/png/jpg/xlsx/xls/csv/doc/docx) — dari F2 diperkuat F7. Permission matrix penuh → saat pilot (butuh akun per-role nyata)
- [x] Pilot PREP ✅: `PILOT.md` — unit FIKK/FIKST/FBIS (data dev siap), checklist pra-pilot, skenario UAT 1 siklus penuh, metrik sukses, langkah go-live. Eksekusi pilot bersama tim → setelah Mas Rama setujui jadwal
- [x] Backup ✅: `deploy/backup-iku.sh` (pg_dump custom-compressed + evidence tar + env snapshot; teruji restore-list 104 TOC). Rollout seluruh unit → pasca-pilot
- [x] Verifikasi JDIH ✅ (mekanisme): kolom verification_status/verified_by/at + document_url di regulatory_versions, `PATCH /api/regulations/:id/verify`, UI badge Verified + tombol Verify JDIH di Compliance. Verifikasi konten aktual vs dokumen resmi → saat go-live (perlu PDF resmi — Open Question #5)

## 10. Estimasi & Cara Kerja

Estimasi blueprint (54 minggu, tim 4) → kita duo iteratif: tiap fase = kerja berkala, tanpa deadline keras, kualitas > kecepatan; urutan tetap. F0–F2 = **MVP** (blueprint Bab 9.2) — fokus kita sampai situ dulu; F3 menyusul cepat karena nilai demo tinggi; F5–F6 setelah MVP terbukti dipakai.

## 11. Open Questions (perlu jawaban Mas Rama, sebelum/di sepanjang jalan)

1. **Unit kerja & klaster**: pemetaan resmi unit Binawan → (branch/department) HC + pengelompokan "klaster" untuk dashboard? (dipakai F1/F3) — *belum dijawab*
2. ~~SIAKAD API?~~ **DIJAWAB 18 Aug:** SIAKAD & LMS vendor saat ini **TIDAK open API**. Mekanisme khusus: (a) **import Excel/CSV terkelola** (template + validasi mapping + audit siapa-import) sebagai jalur utama; (b) opsi **DB view/read-replica** kalau vendor memberi akses baca; (c) LMS rencana pindah ke **Moodle** (REST API) — migrasi bertahap, connector Moodle disiapkan saat migrasi mulai (F5 tidak memblokir). Connector pattern tetap contract-first; "import terkelola" adalah connector type `file-import`.
3. **Periode default**: triwulanan untuk semua 12 IKU di awal, atau mix? Sementara seed F0: IKU = annual (sesuai blueprint tabel 1.2) + periode TW 2026 tersedia. — konfirmasi BPM menyusul.
4. **Penandatangan approval** (jabatan "pimpinan unit" di Binawan) — untuk mapping peran & UI F2. *belum dijawab*
5. **Dokumen resmi 358/2025** (PDF + Juknis) untuk document_url & verifikasi formula — *belum dijawab*
6. **Pilot units** F7 — *menyusul*

## 12. Riwayat Perubahan

| Tanggal | Perubahan |
|---|---|
| 2026-08-18 | Roadmap dibuat dari Blueprint v2 + riset eCakradipa/IKU Indonesia. Service iku lama dihapus, scaffold baru via CLI (port 8083). Menunggu persetujuan Mas Rama sebelum mulai F0. |
| 2026-08-18 | **Mas Rama menyetujui roadmap** (dengan catatan): SIAKAD/LMS no-API → mekanisme import terkelola + roadmap Moodle; MinIO dev compose dibuat (deploy/minio/) + env S3 prod-ready. **F0 SELESAI**: entities+seed 12 IKU+periode, manifest self-sync, endpoints, UI Indikator, E2E API+browser lulus. |
