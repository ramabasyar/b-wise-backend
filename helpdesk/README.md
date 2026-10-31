# B-Wise Starter Kit

Template untuk membuat service baru di ekosistem B-Wise. Sudah termasuk semua boilerplate yang dibutuhkan — tinggal tambah entity dan business logic.

## Apa yang Sudah Ada

| Feature | Status | Lokasi |
|---------|--------|--------|
| **Config** (YAML + .env) | ✅ | `internal/adapter/config/` |
| **Database** (PostgreSQL + GORM) | ✅ | `internal/adapter/database/` |
| **Redis Cache** | ✅ | `internal/adapter/cache/` |
| **Zap Logger** | ✅ | `internal/adapter/logger/` |
| **CORS** (config-driven) | ✅ | `internal/adapter/api/http/middleware/cors.go` |
| **Security Headers** | ✅ | `internal/adapter/api/http/middleware/security.go` |
| **Request ID** | ✅ | `internal/adapter/api/http/middleware/request_id.go` |
| **JWKS Auth** (user + service tokens) | ✅ | `internal/adapter/api/http/middleware/jwks_auth.go` |
| **Permission Check** | ✅ | `internal/adapter/api/http/middleware/permission_check.go` |
| **Migration System** | ✅ | `internal/migrations/` |
| **DTO Pattern** (request/response/mapper) | ✅ | `internal/domain/dto/` |
| **Error Types** | ✅ | `pkg/errors/` |
| **Response Helper** | ✅ | `pkg/response/` |
| **Health Check** | ✅ | `GET /health` |
| **Graceful Shutdown** | ✅ | `cmd/server/main.go` |

## Struktur Folder

```
kit/
├── cmd/server/main.go              # Entry point — edit untuk tambah handler
├── configs/config.yaml             # Config default
├── .env.example                    # Template env vars
├── internal/
│   ├── adapter/
│   │   ├── config/                 # Config loader (Viper)
│   │   ├── database/               # PostgreSQL connection
│   │   ├── cache/                  # Redis cache
│   │   ├── logger/                 # Zap logger
│   │   └── api/http/
│   │       ├── handler/            # HTTP handlers — ADD YOURS HERE
│   │       ├── middleware/         # All middleware (auth, cors, security, etc.)
│   │       └── router/             # Route definitions — ADD YOUR ROUTES HERE
│   ├── domain/
│   │   ├── entity/                 # Domain entities — ADD YOURS HERE
│   │   ├── dto/
│   │   │   ├── request/            # Request DTOs
│   │   │   ├── response/           # Response DTOs
│   │   │   └── mapper/             # Entity ↔ DTO mappers
│   │   └── service/                # Service interfaces
│   └── migrations/                 # Database migrations
├── pkg/
│   ├── errors/                     # AppError types
│   ├── response/                   # JSON response helpers
│   ├── jwt/                        # JWT utilities
│   └── sso/                        # SSO client + cache
```

## Cara Pakai

### 1. Copy starter kit

```bash
# Copy ke lokasi baru
cp -r ~/project/go/starter/kit ~/project/go/b-wise/my-new-service

# Update module name
cd ~/project/go/b-wise/my-new-service
sed -i '' 's|github.com/rama/kit|github.com/rama/b-wise/my-new-service|g' $(find . -name "*.go" -type f)
```

### 2. Setup .env

```bash
cp .env.example .env
# Edit .env dengan values yang benar
```

### 3. Register di SSO

Daftarkan service baru di SSO Service Registry untuk mendapatkan `SERVICE_CLIENT_ID` dan `SERVICE_CLIENT_SECRET`.

### 4. Add Entity

Buat file entity baru, misal `internal/domain/entity/employee.go`:

```go
package entity

type Employee struct {
    Base
    FirstName string `json:"first_name" gorm:"type:varchar(100);not null"`
    LastName  string `json:"last_name" gorm:"type:varchar(100)"`
    Email     string `json:"email" gorm:"type:varchar(255);uniqueIndex;not null"`
    Status    string `json:"status" gorm:"type:varchar(20);default:'active'"`
}

func (Employee) TableName() string { return "employees" }
```

### 5. Add AutoMigrate in main.go

```go
// Uncomment dan tambah entity
err = database.AutoMigrate(db, &entity.Employee{})
```

### 6. Add Routes in router.go

```go
// Di router.Handlers struct:
type Handlers struct {
    Employee *handler.EmployeeHandler
}

// Di SetupWithLogger:
employees := api.Group("/employees")
employees.GET("", handlers.Employee.List)
employees.GET("/:id", handlers.Employee.Get)
employees.POST("", handlers.Employee.Create)
employees.PUT("/:id", handlers.Employee.Update)
employees.DELETE("/:id", handlers.Employee.Delete)
```

### 7. Run

```bash
go build -o bin/service ./cmd/server/
./bin/service
```

## Middleware Chain (Sudah Terpasang)

```
Request → Recovery → CORS → Security Headers → Request ID → Logger → JWKS Auth → Handler
```

- **Recovery**: Catch panics → 500
- **CORS**: Config-driven origins
- **Security**: 11 security headers (HSTS, CSP, etc.)
- **Request ID**: UUID per request, in logs + response header
- **Logger**: Structured JSON logging with request ID
- **JWKS Auth**: Validates JWT via SSO JWKS endpoint (supports both RSA + HMAC)

## Env Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SERVER_PORT` | No | 8081 | HTTP port |
| `DB_HOST` | No | localhost | PostgreSQL host |
| `DB_PORT` | No | 5432 | PostgreSQL port |
| `DB_USER` | No | postgres | DB user |
| `DB_PASSWORD` | No | postgres | DB password |
| `DB_NAME` | No | kit_db | DB name |
| `JWT_SECRET` | Yes | - | Must match SSO |
| `SSO_URL` | No | http://localhost:8080 | SSO service URL |
| `PERMISSION_SERVICE_URL` | No | http://localhost:8082 | Permission Service URL |
| `SERVICE_NAME` | Yes | - | Registered service name |
| `SERVICE_CLIENT_ID` | Yes | - | From SSO Service Registry |
| `SERVICE_CLIENT_SECRET` | Yes | - | From SSO Service Registry |
| `CORS_ALLOWED_ORIGINS` | No | * | Comma-separated origins |
| `LOGGER_LEVEL` | No | info | debug/info/warn/error |
