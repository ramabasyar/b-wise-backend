# Human Capital Service

> Part of **B-Wise** (Binawan Web Integrated Smart Ecosystem)

Service manajemen data karyawan, departemen, dan jabatan.

## Tech Stack

- **Go** 1.25+ with Gin HTTP framework
- **PostgreSQL** — Database `hc_db`
- **GORM** — ORM
- **Redis** — Caching (optional)
- **JWKS Auth** — Token verification via SSO public key

## Architecture

```
cmd/server/main.go          # Entry point
internal/
  domain/
    entity/                 # Employee, Department, Position
    dto/
      request/              # CreateEmployee, UpdateEmployee, etc.
      response/             # EmployeeResponse, PaginationMeta, etc.
      mapper/               # Entity ↔ DTO conversion
    repository/             # Repository interfaces
    service/                # Service interfaces
  adapter/
    api/http/
      handler/              # HTTP handlers
      middleware/            # JWKS auth, permission check, CORS, etc.
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
go build -o bin/hc ./cmd/server
./bin/hc
```

## API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| GET | `/health` | Public | Health check |
| GET | `/api/employees` | `employees.read` | List employees |
| GET | `/api/employees/:id` | `employees.read` | Get employee |
| POST | `/api/employees` | `employees.write` | Create employee |
| PUT | `/api/employees/:id` | `employees.write` | Update employee |
| DELETE | `/api/employees/:id` | `employees.delete` | Delete employee |
| GET | `/api/departments` | `departments.read` | List departments |
| POST | `/api/departments` | `departments.write` | Create department |
| PUT | `/api/departments/:id` | `departments.write` | Update department |
| DELETE | `/api/departments/:id` | `departments.delete` | Delete department |
| GET | `/api/positions` | `positions.read` | List positions |
| POST | `/api/positions` | `positions.write` | Create position |
| DELETE | `/api/positions/:id` | `positions.delete` | Delete position |

See [API_DOCS.md](./API_DOCS.md) for full documentation.

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_PORT` | `8081` | HTTP server port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_NAME` | `hc_db` | Database name |
| `SSO_URL` | `http://localhost:8080` | SSO service URL |
| `PERMISSION_SERVICE_URL` | `http://localhost:8082` | Permission Service URL |
| `SERVICE_NAME` | `human-capital` | Service name for permission check |
| `SERVICE_CLIENT_ID` | — | OAuth2 client ID from SSO registry |
| `SERVICE_CLIENT_SECRET` | — | OAuth2 client secret from SSO registry |

## Authentication Flow

1. User logs in via SSO → receives RS256 JWT
2. Request to `/api/*` with `Authorization: Bearer <token>`
3. JWKS middleware verifies token via SSO public key
4. Permission middleware checks access + specific permission via Permission Service
5. Service tokens (OAuth2) bypass permission checks

*Last updated: 2026-05-20*
