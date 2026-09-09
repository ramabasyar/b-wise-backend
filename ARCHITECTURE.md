# B-Wise Microservices Ecosystem

> **Binawan Web Integrated Smart Ecosystem**
> Dokumentasi arsitektur dan alur aplikasi.

---

## Daftar Isi

1. [Arsitektur Overview](#1-arsitektur-overview)
2. [Service Registry](#2-service-registry)
3. [Alur User Authentication](#3-alur-user-authentication)
4. [Alur Service-to-Service Auth](#4-alur-service-to-service-auth)
5. [Alur Permission & Access Control](#5-alur-permission--access-control)
6. [Alur Frontend Integration](#6-alur-frontend-integration)
7. [Database Schema](#7-database-schema)
8. [Environment Variables](#8-environment-variables)
9. [Quick Start](#9-quick-start)

---

## 1. Arsitektur Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                         B-Wise Ecosystem                         │
│                                                                  │
│  ┌──────────┐     ┌──────────────────────────────────────────┐  │
│  │          │     │              SSO Service (:8080)          │  │
│  │  User /  │────→│  Login • Register • RSA JWT • JWKS      │  │
│  │  Admin   │     │  Service Registry • OAuth2 Tokens        │  │
│  │          │     └────────────┬─────────────────────────────┘  │
│  └──────────┘                  │                                 │
│                                │ User JWT (RS256)                │
│                                │ Service Token (OAuth2)          │
│                                ▼                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │              Permission Service (:8082)                    │   │
│  │  Services • Permissions • Access • User Menu               │   │
│  └────────────┬─────────────────────────────────────────────┘   │
│               │ Verify access + permission                       │
│               ▼                                                  │
│  ┌──────────────────────────┐  ┌──────────────────────────┐     │
│  │  Human Capital (:8081)   │  │  Future Services (:8083+) │     │
│  │  Employees • Departments │  │  Finance • Academic • ... │     │
│  │  Positions               │  │                            │     │
│  └──────────────────────────┘  └──────────────────────────┘     │
│                                                                  │
│  Infrastructure: PostgreSQL (3 DBs) • Redis • RSA Keys          │
└──────────────────────────────────────────────────────────────────┘
```

### Services

| Service | Port | Database | Fungsi |
|---------|------|----------|--------|
| **SSO Service** | 8080 | `sso` | Autentikasi user, manajemen RSA keys, service registry, OAuth2 token |
| **Permission Service** | 8082 | `permission_db` | Manajemen service, permission, user access, user menu |
| **Human Capital** | 8081 | `hc_db` | Data karyawan, departemen, jabatan |

---

## 2. Service Registry

Sebelum backend service bisa berkomunikasi, harus di-register di SSO:

```
Admin → POST /api/admin/service-registry
        {
          "service_name": "human-capital",
          "allowed_scopes": ["human-capital:employees:read", ...],
          "created_by": "admin"
        }
        ↓
SSO generates:
  - client_id: human-capi-e89a8afc
  - client_secret: <random-secret>
        ↓
Admin stores credentials in service's .env file
```

### Service Terdaftar

| Service | Client ID | Scope Pattern |
|---------|-----------|---------------|
| Human Capital | `human-capi-e89a8afc` | `human-capital:*` |
| Permission Service | `permission-1f31a7e7` | `permission:*` |

---

## 3. Alur User Authentication

```
┌────────┐         ┌─────────┐         ┌──────────┐         ┌──────────┐
│  User  │         │ Frontend│         │   SSO    │         │    HC    │
└───┬────┘         └────┬────┘         └────┬─────┘         └────┬─────┘
    │  Login form       │                   │                    │
    │──────────────────→│                   │                    │
    │                   │  POST /auth/login │                    │
    │                   │──────────────────→│                    │
    │                   │                   │ Verify credentials │
    │                   │                   │ Generate RS256 JWT │
    │                   │  { access_token,  │                    │
    │                   │    user, roles }   │                    │
    │                   │←──────────────────│                    │
    │  Show dashboard   │                   │                    │
    │←──────────────────│                   │                    │
    │                   │                   │                    │
    │  Click "Employees"│                   │                    │
    │──────────────────→│                   │                    │
    │                   │  GET /api/employees                    │
    │                   │  Bearer: user_jwt │                    │
    │                   │───────────────────────────────────────→│
    │                   │                   │                    │
    │                   │                   │   1. Verify JWT    │
    │                   │                   │   via JWKS         │
    │                   │                   │   2. Check access  │
    │                   │                   │   via Perm Svc     │
    │                   │                   │   3. Check perm    │
    │                   │                   │   via Perm Svc     │
    │                   │                   │   4. Return data   │
    │                   │                   │                    │
    │                   │  { employees[] }  │                    │
    │                   │←───────────────────────────────────────│
    │  Show employees   │                   │                    │
    │←──────────────────│                   │                    │
```

### Step-by-step

1. **User login** via `POST /api/auth/login` ke SSO
2. **SSO** verify credentials → generate JWT (RS256) → return `access_token` + `user`
3. **Frontend** simpan token, gunakan untuk setiap request ke backend services
4. **Backend service** (HC/Perm) menerima request:
   - JWKS middleware verify JWT signature via SSO public key (`/.well-known/jwks.json`)
   - Extract `user_id`, `email` dari claims
   - Permission check middleware call Permission Service untuk cek access + permission
   - Jika authorized → return data
   - Jika unauthorized → return 401/403

### User JWT Payload

```json
{
  "user_id": 6,
  "email": "admin@example.com",
  "iss": "sso-service",
  "exp": 1779266349,
  "iat": 1779266325
}
```

---

## 4. Alur Service-to-Service Auth

Service-to-service menggunakan **OAuth2 Client Credentials**:

```
┌────────────┐         ┌─────────┐         ┌──────────────┐
│ HC Service │         │   SSO   │         │  Perm Svc    │
└─────┬──────┘         └────┬────┘         └──────┬───────┘
      │                     │                      │
      │ 1. Need to call     │                      │
      │    Perm Service     │                      │
      │                     │                      │
      │ POST /api/oauth/token                      │
      │ { client_id, client_secret,                │
      │   grant_type: "client_credentials" }        │
      │────────────────────→│                      │
      │                     │                      │
      │ { access_token,     │                      │
      │   token_type,       │                      │
      │   expires_in: 900 } │                      │
      │←────────────────────│                      │
      │                     │                      │
      │ 2. Call Perm Svc    │                      │
      │    with service     │                      │
      │    token            │                      │
      │                     │                      │
      │ GET /api/services   │                      │
      │ Bearer: svc_token   │                      │
      │─────────────────────────────────────────────→│
      │                     │                      │
      │ { services[] }      │                      │
      │←─────────────────────────────────────────────│
      │                     │                      │
```

### Service JWT Payload

```json
{
  "service_name": "human-capital",
  "client_id": "human-capi-e89a8afc",
  "scopes": ["human-capital:employees:read", "human-capital:employees:write"],
  "token_type": "service",
  "iss": "sso-service",
  "sub": "human-capital",
  "aud": ["b-wise-services"],
  "exp": 1779267000,
  "iat": 1779266100
}
```

### Scope Pattern

```
{service}:{resource}:{action}

Contoh:
  human-capital:employees:read
  human-capital:employees:write
  human-capital:departments:read
  permission:access:write
```

### Service Token Behavior

- **Service token BYPASS permission checks** — service dianggap trusted
- Expire dalam **15 menit**, harus request ulang
- Signed dengan **RS256** (RSA private key di SSO)
- Di-verify oleh service lain via **JWKS public key**

---

## 5. Alur Permission & Access Control

```
┌─────────────────────────────────────────────────────────┐
│                  Permission Check Flow                    │
│                                                          │
│  Request masuk ke HC Service                             │
│         │                                                │
│         ▼                                                │
│  ┌─ Token ada? ─┐                                       │
│  │ Tidak        │ Ya                                     │
│  │ → 401        │                                        │
│  └──────────────┘                                        │
│         │                                                │
│         ▼                                                │
│  ┌─ Verify JWT via JWKS ─┐                              │
│  │ Invalid → 401         │ Valid                         │
│  └────────────────────────┘                              │
│         │                                                │
│         ▼                                                │
│  ┌─ Service token? ──────┐                              │
│  │ Ya → SKIP perm check  │ User token                    │
│  │ → Return data         │                               │
│  └────────────────────────┘                              │
│         │                                                │
│         ▼                                                │
│  ┌─ CheckAccess() ───────┐                              │
│  │ Call Perm Svc:        │                              │
│  │ GET /api/users/:id/   │                              │
│  │   services/:sid/check │                              │
│  │                       │                              │
│  │ No access → 403       │                              │
│  └───────────────────────┘                              │
│         │                                                │
│         ▼                                                │
│  ┌─ RequirePermission() ┐                               │
│  │ Call Perm Svc:       │                               │
│  │ GET /api/users/:id/  │                               │
│  │   permissions/check  │                               │
│  │                      │                               │
│  │ No permission → 403  │                               │
│  └──────────────────────┘                               │
│         │                                                │
│         ▼                                                │
│    ✅ Return data                                        │
└─────────────────────────────────────────────────────────┘
```

### Permission Levels

| Level | Check | Fungsi |
|-------|-------|--------|
| **Authentication** | JWT valid? | `jwksAuth.RequireAuth()` |
| **Access** | User punya akses ke service? | `permCheck.CheckAccess()` |
| **Permission** | User punya permission spesifik? | `permCheck.RequirePermission("employees.read")` |

### Permission Mapping (HC)

| Endpoint | Permission Required |
|----------|-------------------|
| `GET /api/employees` | `employees.read` |
| `GET /api/employees/:id` | `employees.read` |
| `POST /api/employees` | `employees.write` |
| `PUT /api/employees/:id` | `employees.write` |
| `DELETE /api/employees/:id` | `employees.delete` |
| `GET /api/departments` | `departments.read` |
| `POST /api/departments` | `departments.write` |
| `PUT /api/departments/:id` | `departments.write` |
| `DELETE /api/departments/:id` | `departments.delete` |
| `GET /api/positions` | `positions.read` |
| `POST /api/positions` | `positions.write` |
| `DELETE /api/positions/:id` | `positions.delete` |

---

## 6. Alur Frontend Integration

### Login Flow

```
1. User buka frontend (Next.js)
2. User isi form login (email + password)
3. Frontend POST ke SSO: /api/auth/login
4. SSO return { access_token, user, roles }
5. Frontend simpan token di localStorage/httpOnly cookie
6. Frontend redirect ke dashboard
```

### Menu Rendering Flow

```
1. User login berhasil
2. Frontend GET Permission Service: /api/users/:id/menu
   Authorization: Bearer <user_token>
3. Perm Svc return menu items:
   [
     {
       "service_name": "human-capital",
       "permissions": ["employees.read", "employees.write", ...]
     }
   ]
4. Frontend render sidebar berdasarkan menu items
5. User klik menu → frontend call backend service dengan user token
```

### API Call Flow

```
1. Frontend call backend service
2. Tambah header: Authorization: Bearer <user_token>
3. Backend service verify token + check permission
4. Return data / error
```

---

## 7. Database Schema

### SSO Database (`sso`)

| Tabel | Fungsi |
|-------|--------|
| `users` | Data user (email, password hash, status) |
| `roles` | Role definitions (user, admin, moderator) |
| `user_roles` | User ↔ Role mapping |
| `refresh_tokens` | JWT refresh tokens (TEXT column) |
| `signing_keys` | RSA key pairs untuk JWT signing |
| `service_registries` | Registered microservices (client_id, secret, scopes) |

### Permission Database (`permission_db`)

| Tabel | Fungsi |
|-------|--------|
| `services` | Registered backend services |
| `service_permissions` | Permission definitions per service |
| `service_access` | User ↔ Service mapping (akses atau tidak) |
| `user_permissions` | User ↔ Permission mapping (detail permission) |

### Human Capital Database (`hc_db`)

| Tabel | Fungsi |
|-------|--------|
| `employees` | Data karyawan |
| `departments` | Departemen |
| `positions` | Jabatan |

---

## 8. Environment Variables

### SSO Service (.env)

```env
SERVER_PORT=8080
DB_HOST=localhost
DB_PORT=5432
DB_USER=ssouser
DB_PASSWORD=userlocal
DB_NAME=sso
DB_SSLMODE=disable
REDIS_HOST=localhost
REDIS_PORT=6379
JWT_SECRET=your-jwt-secret
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=Admin@123
ADMIN_USERNAME=admin
```

### Permission Service (.env)

```env
SERVER_PORT=8082
DB_HOST=localhost
DB_PORT=5432
DB_USER=sso
DB_PASSWORD=userlocal
DB_NAME=permission_db
DB_SSLMODE=disable
REDIS_HOST=localhost
REDIS_PORT=6379
SSO_URL=http://localhost:8080
SERVICE_NAME=permission-service
```

### Human Capital (.env)

```env
SERVER_PORT=8081
DB_HOST=localhost
DB_PORT=5432
DB_USER=sso
DB_PASSWORD=userlocal
DB_NAME=hc_db
DB_SSLMODE=disable
REDIS_HOST=localhost
REDIS_PORT=6379
SSO_URL=http://localhost:8080
PERMISSION_SERVICE_URL=http://localhost:8082
SERVICE_NAME=human-capital
SERVICE_CLIENT_ID=human-capi-e89a8afc
SERVICE_CLIENT_SECRET=<from-sso-registry>
```

---

## 9. Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 14+
- Redis (optional, for caching)

### 1. Setup Database

```bash
# Create databases and users
psql -U macbook -c "CREATE USER ssouser WITH PASSWORD 'userlocal';"
psql -U macbook -c "CREATE DATABASE sso OWNER ssouser;"
psql -U macbook -c "CREATE DATABASE permission_db OWNER ssouser;"
psql -U macbook -c "CREATE DATABASE hc_db OWNER ssouser;"
```

### 2. Build & Start SSO

```bash
cd /path/to/sso
cp .env.example .env
# Edit .env with your settings
go build -o bin/sso-api ./cmd/api
./bin/sso-api
```

### 3. Register Services

```bash
# Login as admin first
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"Admin@123"}' \
  | jq -r '.data.access_token')

# Register Human Capital
curl -X POST http://localhost:8080/api/admin/service-registry \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "service_name": "human-capital",
    "allowed_scopes": ["human-capital:employees:read", "human-capital:employees:write"],
    "created_by": "admin"
  }'

# Register Permission Service
curl -X POST http://localhost:8080/api/admin/service-registry \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "service_name": "permission-service",
    "allowed_scopes": ["permission:services:read", "permission:permissions:write"],
    "created_by": "admin"
  }'
```

### 4. Build & Start Permission Service

```bash
cd /path/to/b-wise/permission-service
cp .env.example .env
# Edit .env with SERVICE credentials from step 3
go build -o bin/permission-service ./cmd/server
./bin/permission-service
```

### 5. Build & Start Human Capital

```bash
cd /path/to/b-wise/human-capital
cp .env.example .env
# Edit .env with SERVICE_CLIENT_ID and SERVICE_CLIENT_SECRET from step 3
go build -o bin/hc ./cmd/server
./bin/hc
```

### 6. Setup Permissions

```bash
# Login as admin
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"Admin@123"}' \
  | jq -r '.data.access_token')

# Get permission service token
PERM_TOKEN=$(curl -s -X POST http://localhost:8080/api/oauth/token \
  -H "Content-Type: application/json" \
  -d '{"client_id":"<perm_client_id>","client_secret":"<perm_secret>","grant_type":"client_credentials"}' \
  | jq -r '.data.access_token')

# Seed permissions
curl -X POST "http://localhost:8082/api/services/<hc_service_id>/permissions/seed" \
  -H "Authorization: Bearer $PERM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"permissions":["employees.read","employees.write","employees.delete"]}'

# Grant user access
curl -X POST "http://localhost:8082/api/services/<hc_service_id>/access" \
  -H "Authorization: Bearer $PERM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id":"<user_id>"}'

# Grant user permissions
curl -X POST "http://localhost:8082/api/users/<user_id>/permissions" \
  -H "Authorization: Bearer $PERM_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"permission":"employees.read","service_id":"<hc_service_id>"}'
```

---

## Error Responses

### 401 Unauthorized

```json
{ "error": "authentication required" }
{ "error": "invalid token", "details": "token expired" }
{ "error": "invalid authorization format" }
```

### 403 Forbidden

```json
{ "error": "no access to this service" }
{ "error": "insufficient permissions", "required_permission": "employees.write" }
{ "error": "user token required, service token not allowed" }
```

### 500 Internal Server Error

```json
{ "error": "failed to check permission", "details": "..." }
{ "error": "failed to get service info" }
```

---

## Test Status (36/36 PASSED ✅)

| Service | Tests | Status |
|---------|-------|--------|
| **SSO** | Login, Register, Profile, Logout, Re-login, Service Registry, OAuth2, JWKS | ✅ |
| **Permission** | Auth check, CRUD Services, User Menu, Permission check, Service token | ✅ |
| **Human Capital** | Auth check, CRUD Employees, Departments, Positions, DTO clean, Validation | ✅ |

Run full test: `python3 /tmp/full_test_v2.py` (with exported HC_CID, HC_SECRET, PERM_CID, PERM_SECRET)

---

*Dokumentasi terakhir diupdate: 2026-05-20*
