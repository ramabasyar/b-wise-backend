# Human Capital Service

> Part of **B-Wise** (Binawan Web Integrated Smart Ecosystem) — HRM modern pengganti Talenta, benchmark Frappe HR/HRMS (lihat `HC_DESIGN.md` & `ROADMAP.md`).

## Fitur (per Okt 2026)

- **Onboarding wizard** (`POST /api/onboard`): akun SSO + profil pegawai + NIP otomatis (`BIN-YYYYMM-####`) + movement onboarding + penempatan unit (primary/secondary) + role akses lintas service — sekali jalan, dengan **kompensasi otomatis** (gagal di tengah → user SSO di-rollback) dan **validasi server-side role⊆mapping departemen**.
- **Employee master** kaya (identitas Indonesia: NIK/NPWP/BPJS, employment, exit) + **movements** ber-tanggal (promotion/transfer/status_change/separation — tanpa overwrite tanpa jejak).
- **Struktur organisasi dinamis**: departments tree + 5 kamus admin-managed (employee_types/org_unit_types/employment_levels/academic_ranks/structural_positions) + **mapping departemen→service** (`department_services`, UI "Akses Service per Unit").
- **Auth**: JWKS (RS256) + permission middleware v2 (CheckAccess + RequirePermission, service token via client credentials).

## Tech Stack

- Go 1.25+ / Gin / GORM / PostgreSQL / Redis (optional, fail-open)
- Clean Architecture: handler → DTO → service → repository
- Self-register manifest (`perm.manifest.yaml`) → permission-service saat boot

## Quick Start

```bash
cp .env.example .env   # isi DB + SERVICE_CLIENT_ID/SECRET dari service_registries SSO
go build -o bin/server ./cmd/server
./bin/server
```

## API Endpoints (ringkas)

| Method | Path | Scope | Catatan |
|--------|------|------------|-------------|
| GET | `/health` | public | |
| GET/POST | `/api/employees` | employees.read/write | list (filter+pagination) / manual add (tanpa akun SSO) |
| GET/PUT/DELETE | `/api/employees/:id` | employees.* | detail (educations + movements timeline); PUT hanya field non-employment |
| GET | `/api/employees/stats` | employees.read | |
| PUT | `/api/employees/:id/educations` | employees.write | |
| GET/POST | `/api/movements` | movements.* | promotion/transfer/status_change/separation (transaksional) |
| GET/POST | `/api/onboard/check` | onboard.execute | pre-flight duplikat NIP/NIK/email |
| GET/PUT | `/api/onboard/department-services` | onboard.execute / dictionaries.write | mapping departemen→service (GET wizard, PUT UI Pengaturan Organisasi) |
| POST | `/api/onboard` | onboard.execute | wizard lengkap (SSO + employee + role) |
| GET | `/api/dictionaries` | (access) | 5 kamus dinamis |
| CRUD | `/api/departments`, `/api/designations`, `/api/employment-types`, `/api/grades`, `/api/branches` | *.read/write | master organisasi |
| GET | `/api/org/resolve-approver` | service token only | resolusi approver utk permission-service |

## Dokumentasi

- `API_DOCS.md` — endpoint lengkap
- `HC_DESIGN.md` — desain (Frappe HR-aligned)
- `ROADMAP.md` — fase pengembangan (Fase 2: leave/attendance/ESS; Fase 4: payroll)

*Last updated: 25 Okt 2026 (audit L5 — versi lama masih era "positions")*
