# Permission Service

> Part of **B-Wise** (Binawan Web Integrated Smart Ecosystem)

Service manajemen permission, akses service, dan menu user.

## Tech Stack

- **Go** 1.25+ with Gin HTTP framework
- **PostgreSQL** — Database `permission_db`
- **GORM** — ORM
- **Redis** — Caching (optional)
- **JWKS Auth** — Token verification via SSO public key

## Architecture

```
cmd/server/main.go          # Entry point
internal/
  domain/
    entity/                 # Service, ServicePermission, ServiceAccess, UserPermission
    dto/
      request/              # CreateService, GrantAccess, etc.
      response/             # ServiceResponse, MenuItem, PermissionCheck, etc.
      mapper/               # Entity ↔ DTO conversion
    repository/             # Repository interfaces
    service/                # Service interfaces
  adapter/
    api/http/
      handler/              # HTTP handlers
      middleware/            # JWKS auth, CORS, etc.
      router/               # Route definitions
    config/                 # Viper config
    database/               # PostgreSQL connection
    persistence/postgres/   # GORM repository implementations
  service/                  # Service implementations
configs/config.yaml
```

## Quick Start

```bash
cp .env.example .env
# Edit .env with DB credentials and SSO details
go build -o bin/permission-service ./cmd/server
./bin/permission-service
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (public) |
| **Service Management** | | |
| POST | `/api/services` | Create service |
| GET | `/api/services` | List services |
| GET | `/api/services/:id` | Get service |
| PUT | `/api/services/:id` | Update service |
| DELETE | `/api/services/:id` | Delete service |
| POST | `/api/services/:id/activate` | Activate service |
| POST | `/api/services/:id/deactivate` | Deactivate service |
| **Permission Management** | | |
| POST | `/api/services/:id/permissions` | Create permission |
| POST | `/api/services/:id/permissions/seed` | Seed permissions (batch) |
| GET | `/api/services/:id/permissions` | List permissions |
| DELETE | `/api/permissions/:id` | Delete permission |
| **Access Management** | | |
| POST | `/api/services/:id/access` | Grant service access |
| DELETE | `/api/access/:id` | Revoke access |
| **User Permissions** | | |
| POST | `/api/users/:id/permissions` | Grant user permission |
| GET | `/api/users/:id/permissions` | Get user permissions |
| GET | `/api/users/:id/permissions/check` | Check permission (supports wildcard `*`) |
| POST | `/api/users/:id/permissions/check-batch` | Batch check: body `{service_id, permissions:[..]}` → map hasil (1x query) |
| GET | `/api/users/:id/permissions/service/:sid` | Get perms by service |
| DELETE | `/api/user-permissions/:id` | Revoke permission |
| **Self-Sync (Manifest)** | | |
| POST | `/api/services/self/sync` | Service mendaftarkan/mutakhirkan manifest-nya sendiri (service token only; upsert + deactivate yang hilang; flush menu cache) |
| **Audit Trail** | | |
| GET | `/api/audit-logs` | Jejak grant/revoke assignment (?user_id=&action=&service_id=&limit=&offset=) |
| **Frontend** | | |
| GET | `/api/users/:id/menu` | Get user menu (permissions flat + menu TREE, Redis cache TTL 2m) |
| GET | `/api/users/:id/services` | Get user's services |
| GET | `/api/users/:id/services/:sid/check` | Check service access |
| **Menu Items (CRUD)** | | |
| POST | `/api/menu-items` | Create menu item (service_id, parent_id, label, path, icon, sort_order, required_permission) |
| GET | `/api/menu-items` | List menu items (?service_id= filter) |
| PUT | `/api/menu-items/:id` | Update menu item |
| DELETE | `/api/menu-items/:id` | Delete menu item |

See [API_DOCS.md](./API_DOCS.md) for full documentation.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8082` | HTTP server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_NAME` | `permission_db` | Database name |
| `SSO_URL` | `http://localhost:8080` | SSO service URL |
| `SERVICE_NAME` | `permission-service` | Service name |

## Authentication

All `/api/*` endpoints require `Authorization: Bearer <token>`. Both user and service tokens are accepted.

## Caching (Tahap 2)

- **Cache-aside + fail-open**: jika Redis mati, semua query jatuh ke DB (layanan tetap jalan).
- Key: `menu:{userID}` (JSON menu + permissions), `perm:{userID}:{serviceID}:{permission}` ("1"/"0"), `userp:{userID}` (set pelacak untuk invalidasi).
- TTL 2 menit; **auto-invalidate** saat: grant/revoke access, grant/revoke user permission, assign/revoke role, set role permissions.
- Konfigurasi Redis di `configs/config.yaml` (`redis.host/port/db`); jika `host` kosong → service jalan tanpa cache.

## Wildcard & Snapshot (Tahap 3)

- **Wildcard `*`**: permission `*` (direct maupun via role) meloloskan SEMUA check di service tersebut — cocok untuk superadmin/service token.
- **No-snapshot**: `assign role` TIDAK lagi menulis direct `user_permissions` — permission efektif selalu dihitung union(direct, via role), sehingga **revoke role otomatis mencabut permission turunannya**.
- **Audit trail**: semua grant/revoke (access, permission, role) tercatat di tabel `audit_logs` (actor, action, target, detail, waktu).
- **Cache CheckAccess**: key `access:{userID}:{serviceID}` (TTL 2m) — cek akses service tidak lagi hit DB tiap request; invalidasi otomatis saat grant/revoke access.

## Self-Register Manifest (Tahap 3+)

Service mendeklarasikan permissions & menunya di `perm.manifest.yaml` (root repo service, ter-commit ke git). Saat boot, service mengirim isinya ke `POST /api/services/self/sync` pakai service token — idempotent:

- **Service**: upsert by name (desc/base_url ter-update bila berubah)
- **Permissions**: upsert by name; yang hilang dari manifest → **deactivate** (`is_active=false`, bukan delete — role mapping tetap aman)
- **Menu**: upsert by path (parent dulu, lalu children); yang hilang → deactivate
- Setelah sync: cache `menu:*` semua user di-flush — perubahan instan
- Keamanan: `service_name` di JWT wajib sama dengan `service.name` di manifest (service tidak bisa mengubah milik service lain)
- Opt-out: `sync_enabled: false` di manifest (admin kelola manual via API)

Starter kit sudah include bootstrap (`internal/service/permissionsync`) — best-effort, retry backoff (2s→30s), gagal sync tidak menghentikan service. CLI `bwise create-service` generate manifest default otomatis.

*Last updated: 2026-05-20*
