# Rekomendasi Struktur Project Go - Clean Architecture

## Ringkasan Perubahan

Struktur yang kamu berikan sudah sangat bagus dan production-ready! Rekomendasi ini adalah improvement untuk lebih mengikuti **Clean Architecture** dan **DDD (Domain-Driven Design)** best practices.

## Perbandingan Struktur

### Struktur Kamu (Sudah Sangat Bagus!)
```
internal/
├── domain/           # Entities only
├── repository/       # Interface + Implementation (satu package)
├── service/          # Business logic
├── dto/              # Request/Response DTOs
├── validator/        # Validation logic
└── api/              # Handler, middleware, router
```

### Rekomendasi (Lebih Clean Architecture)
```
internal/
├── domain/           # CORE DOMAIN (Pure, No External Deps)
│   ├── entity/       # Domain entities
│   ├── repository/   # Repository interfaces (CONTRACTS ONLY)
│   └── usecase/      # UseCase interfaces (CONTRACTS ONLY)
├── usecase/          # BUSINESS LOGIC IMPLEMENTATIONS
├── repository/       # REPOSITORY IMPLEMENTATIONS (Infra layer)
├── api/              # HTTP ADAPTER LAYER
│   ├── handler/
│   ├── middleware/
│   ├── router/
│   └── dto/          # DTOs di sini (belongs to API layer)
└── infrastructure/   # SHARED INFRASTRUCTURE
```

## Perbedaan Utama

| Aspect | Struktur Kamu | Rekomendasi |
|--------|---------------|-------------|
| **Domain** | Entities only | Entities + Repository Interfaces + UseCase Interfaces |
| **Repository** | Interface + Impl (satu package) | Interface di domain, Impl terpisah di repository/ |
| **Business Logic** | service/ | usecase/ (lebih jelas intent) |
| **DTOs** | internal/dto/ | internal/api/dto/ (belongs to API layer) |
| **Validator** | internal/validator/ | pkg/validator/ (reusable) |

---

## Detail Rekomendasi Struktur

### 1. Domain Layer (CORE - Pure, No External Dependencies)

```
internal/domain/
├── entity/
│   ├── user.go
│   ├── product.go
│   └── order.go
├── repository/
│   ├── user_repository.go      # INTERFACE ONLY
│   ├── product_repository.go   # INTERFACE ONLY
│   └── order_repository.go     # INTERFACE ONLY
└── usecase/
    ├── user_usecase.go         # INTERFACE ONLY
    ├── product_usecase.go      # INTERFACE ONLY
    └── order_usecase.go        # INTERFACE ONLY
```

**Kenapa:**
- Domain layer harus **PURE** (no external dependencies)
- Hanya berisi **business entities** dan **contracts** (interfaces)
- Bisa di-share ke multiple implementations (HTTP, gRPC, CLI)
- Memudahkan testing dan mocking

**Contoh Code:**

```go
// internal/domain/entity/user.go
package entity

import "time"

type User struct {
    ID        string    `json:"id"`
    Email     string    `json:"email"`
    Password  string    `json:"-"` // Tidak di-expose via JSON
    Name      string    `json:"name"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// internal/domain/repository/user_repository.go
package repository

import (
    "github.com/rama/kit/internal/domain/entity"
)

// UserRepository defines the contract for user data access
// INTERFACE ONLY - no implementation details
type UserRepository interface {
    Create(user *entity.User) error
    FindByID(id string) (*entity.User, error)
    FindByEmail(email string) (*entity.User, error)
    Update(user *entity.User) error
    Delete(id string) error
    List(limit, offset int) ([]*entity.User, int64, error)
}

// internal/domain/usecase/user_usecase.go
package usecase

import (
    "github.com/rama/kit/internal/domain/entity"
)

// UserUseCase defines the contract for user business logic
// INTERFACE ONLY - no implementation details
type UserUseCase interface {
    Register(email, password, name string) (*entity.User, error)
    Login(email, password string) (*entity.User, error)
    GetUser(id string) (*entity.User, error)
    UpdateUser(id string, updates map[string]interface{}) (*entity.User, error)
    DeleteUser(id string) error
}
```

---

### 2. Usecase Layer (Business Logic Implementations)

```
internal/usecase/
├── user/
│   ├── service.go         # Implements domain.usecase.UserUseCase
│   └── service_test.go
├── product/
│   ├── service.go         # Implements domain.usecase.ProductUseCase
│   └── service_test.go
└── order/
    ├── service.go         # Implements domain.usecase.OrderUseCase
    └── service_test.go
```

**Kenapa:**
- **Usecase** = Explicit business intent (lebih jelas daripada "Service")
- Enkapsulasi complex business rules di sini
- Boleh import ke repository layer (implementations)
- Boleh import ke external services (email, SMS, etc.)

**Contoh Code:**

```go
// internal/usecase/user/service.go
package user

import (
    "errors"
    "time"

    "github.com/google/uuid"
    "github.com/rama/kit/internal/domain/entity"
    "github.com/rama/kit/internal/domain/repository"
    "github.com/rama/kit/internal/domain/usecase"
    "golang.org/x/crypto/bcrypt"
)

// UserService implements UserUseCase interface
type UserService struct {
    userRepo repository.UserRepository
}

// NewUserService creates a new UserService instance
func NewUserService(userRepo repository.UserRepository) usecase.UserUseCase {
    return &UserService{
        userRepo: userRepo,
    }
}

// Register registers a new user
func (s *UserService) Register(email, password, name string) (*entity.User, error) {
    // Business logic: Validate email uniqueness
    existing, _ := s.userRepo.FindByEmail(email)
    if existing != nil {
        return nil, errors.New("email already registered")
    }

    // Business logic: Hash password
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        return nil, err
    }

    // Business logic: Create user entity
    user := &entity.User{
        ID:        uuid.New().String(),
        Email:     email,
        Password:  string(hashedPassword),
        Name:      name,
        Status:    "active",
        CreatedAt: time.Now().UTC(),
        UpdatedAt: time.Now().UTC(),
    }

    if err := s.userRepo.Create(user); err != nil {
        return nil, err
    }

    return user, nil
}

// Login authenticates a user
func (s *UserService) Login(email, password string) (*entity.User, error) {
    user, err := s.userRepo.FindByEmail(email)
    if err != nil {
        return nil, errors.New("invalid credentials")
    }

    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
        return nil, errors.New("invalid credentials")
    }

    return user, nil
}
```

---

### 3. Repository Layer (Data Access Implementations)

```
internal/repository/
├── postgres/
│   ├── user_repository.go      # Implements domain.repository.UserRepository
│   ├── product_repository.go   # Implements domain.repository.ProductRepository
│   └── models.go               # GORM models
└── memory/                     # In-memory for testing
    ├── user_repository.go
    └── product_repository.go
```

**Kenapa:**
- **Interface** di domain layer (pure)
- **Implementation** di repository layer (infrastructure)
- Bisa punya multiple implementations (PostgreSQL, MySQL, MongoDB, In-Memory)
- Memudahkan testing dengan in-memory repositories

**Contoh Code:**

```go
// internal/repository/postgres/user_repository.go
package postgres

import (
    "github.com/rama/kit/internal/domain/entity"
    "github.com/rama/kit/internal/domain/repository"
    "gorm.io/gorm"
)

// userRepository implements UserRepository interface
type userRepository struct {
    db *gorm.DB
}

// NewUserRepository creates a new UserRepository instance
func NewUserRepository(db *gorm.DB) repository.UserRepository {
    return &userRepository{db: db}
}

// Create creates a new user
func (r *userRepository) Create(user *entity.User) error {
    // Convert domain entity to GORM model
    model := UserGORM{
        ID:        user.ID,
        Email:     user.Email,
        Password:  user.Password,
        Name:      user.Name,
        Status:    user.Status,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
    }

    return r.db.Create(&model).Error
}

// FindByID finds a user by ID
func (r *userRepository) FindByID(id string) (*entity.User, error) {
    var model UserGORM
    err := r.db.Where("id = ?", id).First(&model).Error
    if err != nil {
        return nil, err
    }

    // Convert GORM model to domain entity
    return model.ToDomain(), nil
}

// internal/repository/postgres/models.go
package postgres

import (
    "time"

    "github.com/rama/kit/internal/domain/entity"
)

// UserGORM represents the GORM model for User table
type UserGORM struct {
    ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
    Email     string    `gorm:"uniqueIndex;type:varchar(255);not null" json:"email"`
    Password  string    `gorm:"type:varchar(255);not null" json:"-"`
    Name      string    `gorm:"type:varchar(255);not null" json:"name"`
    Status    string    `gorm:"type:varchar(50);default:'active'" json:"status"`
    CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the table name for GORM
func (UserGORM) TableName() string {
    return "users"
}

// ToDomain converts GORM model to domain entity
func (m *UserGORM) ToDomain() *entity.User {
    return &entity.User{
        ID:        m.ID,
        Email:     m.Email,
        Password:  m.Password,
        Name:      m.Name,
        Status:    m.Status,
        CreatedAt: m.CreatedAt,
        UpdatedAt: m.UpdatedAt,
    }
}
```

---

### 4. API Layer (HTTP Adapter)

```
internal/api/
├── handler/
│   ├── user_handler.go
│   ├── product_handler.go
│   └── auth_handler.go
├── middleware/
│   ├── auth.go
│   ├── cors.go
│   ├── logger.go
│   └── rate_limit.go
├── router/
│   └── router.go
└── dto/                      # DTOs di API layer
    ├── request/
    │   ├── user_request.go
    │   └── product_request.go
    └── response/
        ├── user_response.go
        └── product_response.go
```

**Kenapa DTOs di API Layer:**
- DTOs adalah **API contract**, bukan domain concept
- Berbeda tergantung protocol (HTTP, gRPC, GraphQL)
- Memudahkan versioning API tanpa mengubah domain

**Contoh Code:**

```go
// internal/api/dto/request/user_request.go
package request

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
    Email    string `json:"email" binding:"required,email"`
    Password string `json:"password" binding:"required,min=6"`
    Name     string `json:"name" binding:"required,min=2"`
}

// UpdateUserRequest represents the request body for updating a user
type UpdateUserRequest struct {
    Name  *string `json:"name" binding:"omitempty,min=2"`
    Status *string `json:"status" binding:"omitempty,oneof=active inactive"`
}

// internal/api/dto/response/user_response.go
package response

import "time"

// UserResponse represents the response body for user
type UserResponse struct {
    ID        string    `json:"id"`
    Email     string    `json:"email"`
    Name      string    `json:"name"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

// internal/api/handler/user_handler.go
package handler

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/rama/kit/internal/api/dto/request"
    "github.com/rama/kit/internal/api/dto/response"
    userUseCase "github.com/rama/kit/internal/domain/usecase"
)

// UserHandler handles HTTP requests for users
type UserHandler struct {
    userUC userUseCase.UserUseCase
}

// NewUserHandler creates a new UserHandler instance
func NewUserHandler(userUC userUseCase.UserUseCase) *UserHandler {
    return &UserHandler{userUC: userUC}
}

// CreateUser handles POST /api/v1/users
func (h *UserHandler) CreateUser(c *gin.Context) {
    var req request.CreateUserRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
        return
    }

    user, err := h.userUC.Register(req.Email, req.Password, req.Name)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    resp := response.UserResponse{
        ID:        user.ID,
        Email:     user.Email,
        Name:      user.Name,
        Status:    user.Status,
        CreatedAt: user.CreatedAt,
        UpdatedAt: user.UpdatedAt,
    }

    c.JSON(http.StatusCreated, resp)
}
```

---

### 5. Infrastructure Layer (Shared Infrastructure)

```
internal/infrastructure/
├── database/
│   └── postgres.go        # DB connection
├── cache/
│   └── redis.go
└── sso/
    └── client.go          # SSO HTTP client
```

**Kenapa:**
- Cross-cutting concerns
- Bisa di-share oleh multiple usecases
- Terpisah dari business logic

---

## pkg/ Changes

```
pkg/
├── database/
│   ├── postgres.go
│   ├── mysql.go
│   └── migration.go
├── cache/
│   ├── redis.go
│   └── cache.go
├── logger/
│   └── logger.go
├── jwt/
│   └── jwt.go
├── validator/              # MOVED from internal/
│   └── validator.go
├── utils/
│   ├── hash.go
│   ├── validator.go
│   └── response.go
└── config/
    └── config.go
```

**Kenapa Pindahkan Validator ke pkg/:**
- Validator adalah **reusable component**
- Bisa dipakai oleh multiple projects
- Tidak spesifik ke business logic

---

## Diagram Dependency Flow

```
┌─────────────────────────────────────────────────────────┐
│                     HTTP / gRPC / CLI                    │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │   API Layer        │
              │  (handler/dto)     │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │   Usecase Layer    │
              │  (business logic)   │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │  Repository Layer   │
              │  (data access)      │
              └──────────┬──────────┘
                         │
                         ▼
              ┌─────────────────────┐
              │   Database / Cache  │
              └─────────────────────┘

Domain Layer (Interfaces) di tengah, di-import oleh semua layers!
```

---

## Keuntungan Struktur Ini

### 1. **Testability**
```go
// Testing dengan mock repository
mockRepo := &MockUserRepository{}
userService := NewUserService(mockRepo)

// Testing dengan in-memory repository
memoryRepo := NewMemoryUserRepository()
userService := NewUserService(memoryRepo)
```

### 2. **Flexibility**
```go
// Bisa ganti implementation tanpa mengubah business logic
type UserService struct {
    userRepo repository.UserRepository  // Bisa PostgreSQL, MySQL, MongoDB
}

// Bisa ganti API tanpa mengubah business logic
type UserHandler struct {
    userUC usecase.UserUseCase  // Bisa HTTP, gRPC, GraphQL
}
```

### 3. **Separation of Concerns**
- **Domain**: Business entities dan contracts
- **Usecase**: Business logic
- **Repository**: Data access
- **API**: HTTP handling

### 4. **Scalability**
- Bisa tambah layer baru tanpa breaking existing code
- Bisa deploy microservices terpisah

---

## Migration Path (Step-by-Step)

### Phase 1: Refactor Domain Layer
1. Buat `internal/domain/entity/` dan pindahkan entities
2. Buat `internal/domain/repository/` untuk interfaces
3. Buat `internal/domain/usecase/` untuk interfaces

### Phase 2: Refactor Repository Layer
1. Buat `internal/repository/postgres/` untuk implementations
2. Buat `internal/repository/memory/` untuk testing
3. Hapus implementations dari `internal/repository/`

### Phase 3: Refactor Usecase Layer
1. Rename `internal/service/` → `internal/usecase/`
2. Update implementations untuk mengimplement interfaces dari domain

### Phase 4: Refactor API Layer
1. Pindahkan `internal/dto/` → `internal/api/dto/`
2. Update handlers untuk menggunakan usecase layer

### Phase 5: Refactor pkg/
1. Pindahkan `internal/validator/` → `pkg/validator/`

---

## Kesimpulan

Struktur yang kamu berikan **sudah sangat bagus** dan **production-ready**! Rekomendasi ini hanya untuk membuatnya **lebih mengikuti Clean Architecture** dan **memudahkan testing/scalability**.

**Kalau kamu mau praktis dan cepat**, struktur kamu sudah cukup! ✅

**Kalau kamu mau strict Clean Architecture**, pakai rekomendasi ini! 🎯

Terserah preferensi Mas Ram! 🤔
