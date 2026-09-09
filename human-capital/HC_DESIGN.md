# HC_DESIGN — Human Capital Service Design (Frappe HR-Aligned)

> **Status:** v1.0 — acuan implementasi Core HR rebuild.
> **Benchmark:** Frappe HR / HRMS (frappe/hrms + erpnext HR doctypes), dipelajari langsung dari source schema (Employee 109 field, Department, Designation, Employment Type, Employee Promotion/Transfer/Separation, Employee Property History).
> **Tanggal:** 2026-08-18
> **Prinsip:** *struktur mengikuti Frappe HR, implementasi mengikuti konvensi B-Wise (Go + Gin + GORM + Clean Architecture + permission-service).*

---

## 1. Apa yang Dipelajari dari Frappe HR (dan diadopsi)

| Konsep Frappe HR | Adopsi di B-Wise HC | Catatan |
|---|---|---|
| **Employee** = satu master kaya dengan tab (basic / employment / contact / personal / education / history / exit) | Entity `employees` dengan grouping kolom yang sama (lihat §3.1) | Bukan tabel terpisah per aspek — satu aggregate |
| Status karyawan: `Active / Inactive / Left` + `relieving_date` | `active / inactive / suspended / left` + field exit | Tambah `suspended` (skors — lazim kampus) |
| **Department** = tree (`parent_department`, `is_group`) | `departments.parent_id` + `is_group` + `code` + `head_employee_id` | Kampus butuh code & kepala unit; tree dipakai fakultas→prodi |
| **Designation** (jabatan) terpisah dari department | Entity `designations` (rename dari "positions") | Istilah konsisten Frappe; `level` dipertahankan (golongan/eselon) |
| **Employment Type** master (Full-time/Contract/Probation/Intern) | `employment_types` seeded: PNS, PPPK, Tetap Yayasan, PKWT, Dosen Luar Biasa, Magang | Konteks Indonesia |
| **Grade** master | `grades` (I–XVII / golongan PNS / jenjang fungsional) | |
| **Branch** (unit bisnis) | `branches` → Fakultas / Lembaga / Unit | Kampus = multi-unit |
| **Employee Promotion / Transfer** = dokumen ber-tanggal dengan child table **Employee Property History** (`property, fieldname, current, new`) | **`employee_movements`** (unified) + child `movement_details` (`property, fieldname, old_value, new_value`) | Frappe pakai 3 doctype terpisah; kita unify satu tabel + `movement_type` — semantics identik, query lebih mudah |
| **Employee Separation** (exit + checklist + exit interview) | `employee_movements` type `separation` + field exit di employee | Checklist clearance → fase offboarding (ROADMAP Fase 3) |
| `internal_work_history` di Employee (auto dari promotion/transfer) | TIDAK duplikat — dibaca dari `employee_movements` (satu sumber kebenaran) | |
| `reports_to` (self-link Employee) | `employees.reports_to` — fondasi approval cuti/lembur Fase 2 | |
| `attendance_device_id` (integrasi mesin fingerprint) | field disiapkan sekarang, dipakai Fase 2 | Future-proof |
| `holiday_list` link | defer ke Fase 2 (leave) | |
| Education child table (`education`) | `employee_educations` (ijazah dosen penting utk kampus) | |
| Bank account, salary mode | defer ke Fase 4 payroll — field dasar bank disiapkan | |
| India-specific (PF/ESI) → **diganti** konteks Indonesia: **NIK, NPWP, BPJS KS/TK, passport** | field identitas Indonesia di tab personal | |

## 2. Scope Rebuild Ini (Core HR)

**Dibangun:**
1. Master org: `departments` (tree), `designations`, `employment_types`, `grades`, `branches` (+CRUD API)
2. `employees` kaya (Frappe-aligned, konteks Indonesia) + `employee_educations` (child)
3. `employee_movements` + `movement_details` — promotion / transfer / status_change tercatat sebagai dokumen riwayat, **mengubah employee secara transaksional** (tidak ada overwrite tanpa jejak)
4. Onboarding wizard API (dipertahankan dari versi lama — create SSO user + employee + role + movement `onboarding` otomatis)
5. `perm.manifest.yaml` + **self-register ke permission-service saat boot** (port package `permissionsync` dari starter-kit) — tidak perlu seed manual lagi
6. Seed master reference (employment types, grades contoh) — **tanpa data dummy karyawan**

**Tidak dibangun (jangan scope creep — lihat ROADMAP.md):**
Leave, attendance, shift, payroll, dokumen upload (MinIO), offboarding checklist, ESS, performance, recruitment.

**Dihapus (disetujui Mas Rama — dummy):** seluruh implementasi lama (entity `Position`, service, handler, router lama). Tabel lama di DB akan di-drop & migrate ulang (`hcdb`).

## 3. Data Model

### 3.1 `employees` (satu aggregate, kolom digroup mengikuti tab Frappe)

```text
-- identification & identity
id (uuid pk), user_id (unique, nullable — FK SSO), employee_number (NIP, unique)
-- names (Frappe: first/middle/last + employee_name)
first_name*, middle_name, last_name, full_name* (generated di service)
gender (male|female), birth_date, marital_status, blood_group
-- identitas Indonesia (adaptasi Frappe personal tab)
nik (16 digit, unique nullable), npwp, bpjs_kesehatan_no, bpjs_tk_no, passport_number
-- contact (Frappe contact tab)
personal_email, work_email, phone, emergency_contact_name, emergency_contact_phone, emergency_relation
-- address
current_address, permanent_address
-- employment (Frappe employment tab)
employment_type_id → employment_types
department_id → departments
designation_id → designations
grade_id → grades
branch_id → branches
reports_to → employees.id (self-link)
joined_date*, scheduled_confirmation_date, final_confirmation_date,
contract_end_date, notice_days, retirement_date
-- employment status
status (active|inactive|suspended|left)
-- exit (Frappe exit tab; terisi saat separation)
resignation_letter_date, relieving_date, reason_for_leaving
-- misc
attendance_device_id, photo_url, bio
-- audit (standar B-Wise)
created_by, updated_by, created_at, updated_at, deleted_at (soft delete)
```

### 3.2 `departments` (tree — Frappe Department)

```text
id, code* (unique), name*, description, parent_id → departments (nullable, root),
is_group bool (node punya anak — dipakai UI tree), head_employee_id → employees (nullable),
is_active, created_by/updated_by/at, deleted_at
```

### 3.3 `designations` (Frappe Designation + level)

```text
id, code (unique), name*, description, level int (1=tertinggi — eselon/golongan),
is_active, timestamps + soft delete
```

### 3.4 `employment_types`

```text
id, name* (unique), description, is_default bool, sort_order,
is_active, timestamps + soft delete
```

### 3.5 `grades`

```text
id, name* (unique, mis. "III/a", "IX — Ahli Pertama"), description, sort_order,
is_active, timestamps + soft delete
```

### 3.6 `branches` (Frappe Branch → Fakultas/Lembaga/Unit)

```text
id, code (unique), name*, description, is_active, timestamps + soft delete
```

### 3.7 `employee_educations` (child, Frappe education table)

```text
id, employee_id → employees (cascade),
level (SMA/S1/S2/S3/Profesi), institution, major, graduation_year,
certificate_no, timestamps
```

### 3.8 `employee_movements` + `movement_details` (★ pola Frappe Property History)

```text
employee_movements:
  id, employee_id* → employees,
  movement_type (onboarding|promotion|transfer|status_change|separation),
  effective_date*, reference_no (surat keputusan), notes,
  created_by, created_at, updated_at

movement_details:                     -- padanan Employee Property History
  id, movement_id → employee_movements (cascade),
  fieldname* (nama field yang berubah, mis. designation_id / status),
  property (label human, mis. "Jabatan"),
  old_value, new_value
```

**Aturan aplikasi movement (service layer, satu DB transaction):**
- Insert movement + details (snapshot old → new) **lalu** update kolom employee terkait.
- Kolom yang boleh diubah via movement (whitelist): `designation_id, department_id, grade_id, branch_id, employment_type_id, reports_to, status, joined_date`(? tidak), exit fields via separation.
- `separation` wajib: `relieving_date` + set `status=left`; menolak kalau employee sudah `left`.
- Riwayat = baca `employee_movements` (tidak ada tabel history duplikat di employee).

## 4. Permission Scopes (`perm.manifest.yaml`)

```yaml
service: human-capital (sync_enabled: true)
permissions:
  employees.read|write|delete
  departments.read|write|delete
  designations.read|write|delete
  employment_types.read|write
  grades.read|write
  branches.read|write
  movements.read|write
  onboard.execute   (wizard — khusus admin SDM)
menu:
  Human Capital:
    - Employees        /dashboard/human-capital/employees      employees.read
    - Onboard          /dashboard/human-capital/onboard        onboard.execute
    - Movements        /dashboard/human-capital/movements      movements.read
    - Departments      /dashboard/human-capital/departments    departments.read
    - Designations     /dashboard/human-capital/designations   designations.read
    - Employment Types /dashboard/human-capital/employment-types employment_types.read
    - Grades           /dashboard/human-capital/grades         grades.read
    - Branches         /dashboard/human-capital/branches       branches.read
```

> Scope lama `positions.*` **deprecated** (ganti `designations.*`). Menu/service registry permission_test akan di-upsert oleh self-sync (yang hilang dari manifest → deactivate otomatis).

## 5. API Endpoints (ringkas)

| Method | Path | Scope | Catatan |
|---|---|---|---|
| GET | `/api/employees` | employees.read | pagination + filter (status, department_id, q: nama/NIP/email) |
| POST | `/api/employees` | employees.write | manual add (tanpa akun SSO — user_id nullable) |
| GET | `/api/employees/:id` | employees.read | termasuk educations + movements (timeline) |
| PUT | `/api/employees/:id` | employees.write | update field non-employment (kontak, personal, alamat, foto) — perubahan employment HARUS via movement |
| DELETE | `/api/employees/:id` | employees.delete | soft delete (bukan resign — resign = movement separation) |
| GET/POST/PUT/DELETE | `/api/departments` … | departments.* | GET list = tree mode (`?tree=true`) |
| GET/POST/PUT/DELETE | `/api/designations`, `/api/employment-types`, `/api/grades`, `/api/branches` | sesuai scope | CRUD standar, list tanpa pagination (data kecil) |
| GET | `/api/employees/:id/movements` | movements.read | riwayat + details |
| GET | `/api/movements` | movements.read | semua (filter employee/type/periode) |
| POST | `/api/movements` | movements.write | buat promotion/transfer/status_change/separation (transaksional) |
| POST | `/api/onboard` | onboard.execute | wizard lama dipertahankan: SSO user + employee + role + movement onboarding |
| GET | `/health` | — | |

## 6. Arsitektur & Konvensi (konsisten dengan B-Wise)

```
cmd/server/main.go
internal/
  domain/entity/        — entities di atas (pure, GORM tags)
  domain/repository/    — interfaces repo
  domain/service/       — business logic (employee, org, movement, onboarding)
  domain/dto/           — request/response DTO + mapper
  adapter/persistence/postgres/  — repo impl (GORM)
  adapter/api/http/handler/      — handler per domain
  adapter/api/http/middleware/   — KEEP: jwks_auth, permission_check, cors, dll
  adapter/api/http/router/       — wiring route + middleware stack standar
  adapter/{config,database,cache,logger}/  — KEEP dari versi lama
  service/permissionsync/        — PORT dari starter-kit (self-register manifest)
perm.manifest.yaml      — sumber kebenaran permission (root service)
```

- Response shape konsisten: `{ success, data, meta?{page,page_size,total} }` (pkg/response existing)
- Validation di DTO request (binding tags) — bukan di handler
- Employee number (NIP): manual input; kalau kosong → auto-generate `SEQ-YYYYMM-####` (naming series ala Frappe, tabel counter)
- Semua write: audit `created_by/updated_by` dari JWT (user_id claim)
- Soft delete konsisten (gorm.DeletedAt)

## 7. Frontend (next/b-wise) — dampak

- Sidebar HC: Employees, Onboard, **Movements** (baru), Departments, **Designations** (ganti Positions), **Employment Types, Grades, Branches** (baru)
- Halaman employees: tabel lebih kaya (NIP, nama, jabatan+grade, departemen+branch, tipe, status) + form create/edit bertab (Employment / Personal / Kontak / Alamat) + detail: timeline movements
- Halaman positions → designations (rename + tanpa department binding)
- Master pages baru (employment types/grades/branches): tabel kecil + dialog CRUD (pola brand component)
- ONBOARDING wizard: flow sama (akun SSO → profil → akses) + field baru (employment type, grade, branch)

## 8. Definition of Done (rebuild ini)

- [ ] Backend compile + AutoMigrate bersih (drop tabel lama di hcdb — dummy disetujui dihapus)
- [ ] Self-sync manifest jalan: services + 17 permission + menu terdaftar di permission_test saat boot
- [ ] CRUD API semua entity via curl (happy path) + movement transaksional terverifikasi (employee berubah + riwayat tercatat)
- [ ] Onboarding wizard E2E: create user SSO + employee + movement + role
- [ ] Frontend halaman HC ter-update & E2E login rama (super admin) semua menu HC kebuka
- [ ] ROADMAP.md fase 1 di-update (rebuild done), MEMORY tercatat
