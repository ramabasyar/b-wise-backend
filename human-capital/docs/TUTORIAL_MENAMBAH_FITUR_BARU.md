# Tutorial Lengkap: Cara Menambah Fitur Baru di Starter Kit

**Tanggal**: 2026-04-24
**Oleh**: Em (AI Assistant)
**Target**: Mas Ram (Pemilik Project)

---

## 📋 Apa yang Akan Kamu Pelajari?

Tutorial ini akan mengajarkan kamu **step-by-step** cara menambah fitur baru (CRUD entity) di Starter Kit dengan Clean Architecture.

**Contoh yang akan kita pelajari**: Membuat fitur **Product Management** dengan:
- Create Product (Buat produk baru)
- Get Product (Ambil data produk)
- List Products (Daftar produk dengan pagination & filter)
- Update Product (Update data produk)
- Delete Product (Hapus produk)

---

## 🎯 Prerequisite

Sebelum memulai, pastikan kamu sudah paham:
1. **Go Language** - Dasar Go (struct, interface, function)
2. **Clean Architecture** - Konsep layer separation
3. **HTTP REST API** - GET, POST, PUT, DELETE
4. **SQL Database** - Dasar query SQL
5. **Gin Framework** - HTTP router untuk Go

---

## 🏗️ Mengenal Struktur Project

Sebelum mulai, mari kita lihat struktur project Starter Kit:

```
internal/
├── domain/                    # DOMAIN LAYER (CORE)
│   ├── entity/                # Domain Entities (PURE)
│   │   ├── user.go
│   │   └── user_test.go
│   ├── repository/            # Repository Interfaces (CONTRACTS)
│   │   └── user_repository.go
│   └── usecase/               # Usecase Interfaces (CONTRACTS)
│       └── user_usecase.go
│
├── usecase/                   # USECASE LAYER (BUSINESS LOGIC)
│   ├── user/
│   │   └── service.go         # Implements UserUseCase
│   └── product/               # Folder baru untuk product
│
├── repository/                # REPOSITORY LAYER (DATA ACCESS)
│   ├── postgres/              # PostgreSQL Implementations
│   │   ├── user_repository.go
│   │   └── models.go          # GORM Models (UserGORM, ProductGORM)
│   └── memory/                # In-memory (untuk testing)
│
├── adapter/                   # ADAPTER LAYER (EXTERNAL INTERFACES)
│   ├── api/http/
│   │   ├── handler/           # HTTP Handlers (Controllers)
│   │   │   ├── user.go
│   │   │   └── health.go
│   │   ├── middleware/        # HTTP Middleware
│   │   └── router/            # Route Configuration
│   └── config/                # Configuration
│
├── infrastructure/            # INFRASTRUCTURE LAYER (SHARED)
│   ├── database/
│   │   └── database.go        # DB Connection & AutoMigrate
│   ├── cache/
│   │   └── redis.go
│   ├── logger/
│   │   └── zap.go
│   └── sso/
│       └── client.go
│
└── api/dto/                   # DATA TRANSFER OBJECTS
    ├── request/
    │   ├── user_request.go
    │   └── product_request.go  # Request DTOs
    └── response/              # Response DTOs
```

---

## 📚 Konsep Utama Clean Architecture

### 1. **Domain Layer** (Core - Pure)
- **Location**: `internal/domain/`
- **Purpose**: Mendefinisikan business entities dan contracts (interfaces)
- **Rule**: TIDAK boleh ada external dependencies (GORM, Gin, database driver)
- **Contains**:
  - `entity/` - Domain entities (User, Product, dll)
  - `repository/` - Repository interfaces (contracts untuk data access)
  - `usecase/` - Usecase interfaces (contracts untuk business logic)

### 2. **Usecase Layer** (Business Logic)
- **Location**: `internal/usecase/`
- **Purpose**: Implementasi business logic
- **Rule**: Boleh depend ke domain layer, TIDAK boleh depend ke infrastructure
- **Contains**: Implementasi dari `domain/usecase/` interfaces

### 3. **Repository Layer** (Data Access)
- **Location**: `internal/repository/`
- **Purpose**: Implementasi data access (SQL queries)
- **Rule**: Boleh depend ke domain layer dan infrastructure (GORM, database driver)
- **Contains**: Implementasi dari `domain/repository/` interfaces

### 4. **Adapter Layer** (External Interfaces)
- **Location**: `internal/adapter/`
- **Purpose**: Handle external communication (HTTP, gRPC, etc)
- **Rule**: Boleh depend ke usecase layer dan domain layer
- **Contains**: Handlers, Middleware, Routers

### 5. **Infrastructure Layer** (Cross-Cutting Concerns)
- **Location**: `internal/infrastructure/`
- **Purpose**: Shared infrastructure services (database, cache, logger)
- **Rule**: Bisa digunakan oleh semua layer

---

## 🎓 12 Step Menambah Fitur Baru

Berikut adalah 12 step yang harus diikuti untuk menambah fitur baru (entity baru) dengan CRUD lengkap.

---

## STEP 1: Buat Domain Entity 📝

**File**: `internal/domain/entity/product.go`

**Apa itu Entity?**
Entity adalah representasi dari business object. Ini PURE artinya:
- Tidak ada GORM tags
- Tidak ada external dependencies
- Hanya business logic dan data structure

**Mengapa Entity harus PURE?**
Agar domain layer tidak terikat ke teknologi tertentu. Jika besok kamu ganti database dari PostgreSQL ke MongoDB, entity tidak perlu diubah.

**Contoh Code**:

```go
package entity

import "time"

// Product represents a product entity (PURE - no GORM tags, no external deps)
type Product struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Price       float64   `json:"price"`
	Stock       int       `json:"stock"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Product status constants
const (
	ProductStatusActive   = "active"
	ProductStatusInactive = "inactive"
	ProductStatusDraft    = "draft"
	ProductStatusArchived = "archived"
)

// Product category constants
const (
	ProductCategoryElectronics = "electronics"
	ProductCategoryClothing    = "clothing"
	ProductCategoryFood        = "food"
	ProductCategoryOther       = "other"
)

// IsActive returns true if product is active
func (p *Product) IsActive() bool {
	return p.Status == ProductStatusActive
}

// IsAvailable returns true if product is in stock
func (p *Product) IsAvailable() bool {
	return p.Stock > 0 && p.IsActive()
}
```

**Penjelasan**:
- **Struct Tags `json:"..."`**: Hanya untuk JSON serialization, bukan database
- **Constants**: Mendefinisikan valid values untuk status dan category
- **Methods**: Business logic methods (`IsActive()`, `IsAvailable()`)
- **NO GORM tags**: Entity tidak tahu tentang database

---

## STEP 2: Buat Repository Interface 🔄

**File**: `internal/domain/repository/product_repository.go`

**Apa itu Repository Interface?**
Repository interface adalah **contract** yang mendefinisikan bagaimana data akan diakses. Ini adalah ABSTRAKSI bukan implementasi.

**Mengapa butuh Interface?**
Agar usecase layer tidak terikat ke implementasi spesifik (PostgreSQL, MySQL, MongoDB). Nanti bisa dengan mudah diganti.

**Contoh Code**:

```go
package repository

import (
	"github.com/rama/kit/internal/domain/entity"
)

// ProductRepository defines the interface for product data access
type ProductRepository interface {
	// Create creates a new product
	Create(product *entity.Product) error

	// FindByID finds a product by ID
	FindByID(id string) (*entity.Product, error)

	// FindAll finds all products with filters
	FindAll(filter ProductFilter) ([]*entity.Product, int64, error)

	// Update updates a product
	Update(product *entity.Product) error

	// Delete deletes a product (soft delete)
	Delete(id string) error

	// CountByStatus counts products by status
	CountByStatus(status string) (int64, error)
}

// ProductFilter defines filter options for product queries
type ProductFilter struct {
	Category string
	Status   string
	Search   string
	Page     int
	Limit    int
}
```

**Penjelasan**:
- **Interface methods**: Mendefinisikan apa yang BISA dilakukan (Create, Find, Update, Delete)
- **Parameters & Return Types**: Menggunakan domain entity `*entity.Product`, bukan GORM model
- **Filter struct**: Struktur untuk query parameters (pagination, search, filter)
- **NO implementation**: Hanya signature method, tidak ada code logic

---

## STEP 3: Buat Usecase Interface 🎯

**File**: `internal/domain/usecase/product_usecase.go`

**Apa itu Usecase Interface?**
Usecase interface adalah **contract** untuk business logic. Ini mendefinisikan apa operasi yang bisa dilakukan dari business perspective.

**Perbedaan dengan Repository Interface**:
- **Repository Interface**: Dari data access perspective (CRUD database)
- **Usecase Interface**: Dari business perspective (validasi, rules, logic)

**Contoh Code**:

```go
package usecase

import "github.com/rama/kit/internal/domain/entity"

// ProductUseCase defines the interface for product business logic
type ProductUseCase interface {
	// CreateProduct creates a new product
	CreateProduct(creatorID, name, description string, price float64, stock int, category, status string) (*entity.Product, error)

	// GetProduct gets a product by ID
	GetProduct(id string) (*entity.Product, error)

	// ListProducts lists products with filters
	ListProducts(filter ProductFilterRequest) ([]*entity.Product, int64, error)

	// UpdateProduct updates a product
	UpdateProduct(id string, updaterID string, updates map[string]interface{}) (*entity.Product, error)

	// DeleteProduct deletes a product
	DeleteProduct(id string) error

	// GetProductStats gets product statistics
	GetProductStats() (*ProductStats, error)
}

// ProductFilterRequest defines filter options for product list
type ProductFilterRequest struct {
	Category string
	Status   string
	Search   string
	Page     int
	Limit    int
}

// ProductStats represents product statistics
type ProductStats struct {
	Total      int64
	Active     int64
	Inactive   int64
	Draft      int64
	Archived   int64
	TotalStock int
}
```

**Penjelasan**:
- **Method names**: Menggunakan business language (`CreateProduct`, bukan `Create`)
- **Parameters**: Terpisah per field (bukan struct entity) untuk explicit validation
- **Return types**: Domain entity atau struct khusus (ProductStats)
- **Request/Response structs**: Struct khusus untuk request/response

---

## STEP 4: Buat GORM Model 🗄️

**File**: `internal/repository/postgres/models.go` (ADD di file yang sudah ada)

**Apa itu GORM Model?**
GORM Model adalah representasi database table. Ini adalah IMPLEMENTATION untuk PostgreSQL.

**Mengapa terpisah dari Entity?**
- Entity = Business concept (PURE)
- GORM Model = Database representation ( teknologi spesifik)
- Converter functions = Mentranslate antara Entity dan Model

**Contoh Code (tambah di models.go)**:

```go
// ProductGORM represents Product model for GORM (separate from domain entity)
type ProductGORM struct {
	ID          string         `gorm:"primaryKey;size:36"`
	Name        string         `gorm:"not null;size:200"`
	Description string         `gorm:"type:text"`
	Price       float64        `gorm:"not null"`
	Stock       int            `gorm:"not null;default:0"`
	Category    string         `gorm:"size:50"`
	Status      string         `gorm:"default:'active';size:20"`
	CreatedBy   string         `gorm:"column:created_by;size:36"`
	UpdatedBy   string         `gorm:"column:updated_by;size:36"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// TableName specifies the table name
func (ProductGORM) TableName() string {
	return "products"
}

// productEntityToModel converts domain entity to GORM model
func productEntityToModel(product *entity.Product) *ProductGORM {
	return &ProductGORM{
		ID:          product.ID,
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		Stock:       product.Stock,
		Category:    product.Category,
		Status:      product.Status,
		CreatedBy:   product.CreatedBy,
		UpdatedBy:   product.UpdatedBy,
		CreatedAt:   product.CreatedAt,
		UpdatedAt:   product.UpdatedAt,
	}
}

// productModelToEntity converts GORM model to domain entity
func productModelToEntity(model *ProductGORM) *entity.Product {
	return &entity.Product{
		ID:          model.ID,
		Name:        model.Name,
		Description: model.Description,
		Price:       model.Price,
		Stock:       model.Stock,
		Category:    model.Category,
		Status:      model.Status,
		CreatedBy:   model.CreatedBy,
		UpdatedBy:   model.UpdatedBy,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}
}
```

**Penjelasan**:
- **GORM Tags**: Mendefinisikan column types, constraints, indexes
- **DeletedAt**: Untuk soft delete (GORM feature)
- **Converter functions**: `Entity → Model` dan `Model → Entity`
- **TableName()**: Mendefinisikan nama table di database

---

## STEP 5: Buat Repository Implementation 💾

**File**: `internal/repository/postgres/product_repository.go`

**Apa itu Repository Implementation?**
Ini adalah implementasi dari `domain/repository/ProductRepository` interface menggunakan GORM untuk PostgreSQL.

**Flow Data Access**:
```
Usecase → calls repository method → Repository → GORM → SQL → Database
```

**Contoh Code**:

```go
package postgres

import (
	"github.com/google/uuid"
	"github.com/rama/kit/internal/domain/entity"
	"github.com/rama/kit/internal/domain/repository"
	"gorm.io/gorm"
)

// productRepository implements ProductRepository interface
type productRepository struct {
	db *gorm.DB
}

// NewProductPersistence creates a new product repository
func NewProductPersistence(db *gorm.DB) repository.ProductRepository {
	return &productRepository{db: db}
}

// Create creates a new product
func (r *productRepository) Create(product *entity.Product) error {
	model := productEntityToModel(product)
	return r.db.Create(model).Error
}

// FindByID finds a product by ID
func (r *productRepository) FindByID(id string) (*entity.Product, error) {
	var model ProductGORM
	err := r.db.Where("id = ?", id).First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return productModelToEntity(&model), nil
}

// FindAll finds all products with filters
func (r *productRepository) FindAll(filter repository.ProductFilter) ([]*entity.Product, int64, error) {
	var models []ProductGORM
	var total int64

	// Build query
	query := r.db.Model(&ProductGORM{})

	// Apply filters
	if filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Search != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?",
			"%"+filter.Search+"%", "%"+filter.Search+"%")
	}

	// Count total (before pagination)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and fetch
	offset := (filter.Page - 1) * filter.Limit
	if err := query.Offset(offset).Limit(filter.Limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}

	// Convert models to entities
	products := make([]*entity.Product, len(models))
	for i, model := range models {
		products[i] = productModelToEntity(&model)
	}

	return products, total, nil
}

// Update updates a product
func (r *productRepository) Update(product *entity.Product) error {
	model := productEntityToModel(product)
	return r.db.Save(model).Error
}

// Delete deletes a product (soft delete)
func (r *productRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&ProductGORM{}).Error
}

// CountByStatus counts products by status
func (r *productRepository) CountByStatus(status string) (int64, error) {
	var count int64
	if status == "" {
		// Count all products
		err := r.db.Model(&ProductGORM{}).Count(&count).Error
		return count, err
	}
	// Count by status
	err := r.db.Model(&ProductGORM{}).Where("status = ?", status).Count(&count).Error
	return count, err
}
```

**Penjelasan**:
- **Constructor**: `NewProductPersistence()` membuat instance repository
- **Create**: Convert entity → model, lalu `db.Create(model)`
- **FindByID**: Query by ID, convert model → entity, return entity
- **FindAll**: Build query dengan filters, apply pagination, count total, convert models → entities
- **Update**: Convert entity → model, lalu `db.Save(model)`
- **Delete**: Soft delete dengan `db.Delete()`
- **CountByStatus**: Count rows berdasarkan status

**GORM Queries**:
- `db.Where("column = ?", value)` - WHERE clause
- `db.Limit(n).Offset(m)` - Pagination
- `db.Count(&total)` - Count rows
- `db.Model(&Model{})` - Start query from model

---

## STEP 6: Update Database AutoMigrate 🔄

**File**: `internal/infrastructure/database/database.go`

**Apa itu AutoMigrate?**
AutoMigrate adalah fitur GORM yang otomatis create/migrate database tables berdasarkan GORM models.

**Kenapa perlu update?**
Karena kita baru saja membuat `ProductGORM` model, jadi harus ditambahkan ke AutoMigrate agar table `products` otomatis dibuat saat server start.

**Contoh Code (update existing code)**:

```go
// AutoMigrate runs auto-migration
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&postgres.UserGORM{},     // Existing
		&postgres.ProductGORM{},  // ADD THIS
	)
}
```

**Penjelasan**:
- GORM akan:
  1. Cek jika table `products` sudah ada → skip
  2. Jika belum ada → create table dengan semua columns
  3. Jika ada tapi schema berubah → alter table
- **Safe to run multiple times** - Tidak akan duplicate data

---

## STEP 7: Buat Request/Response DTOs 📦

**File**: `internal/api/dto/request/product_request.go`

**Apa itu DTO (Data Transfer Object)?**
DTO adalah struct khusus untuk request/response HTTP. Bukan domain entity.

**Mengapa perlu DTO?**
- **Validation**: Boleh pakai validation tags (`required`, `email`, dll)
- **Security**: Hanya expose fields yang perlu saja
- **Flexibility**: Request structure bisa beda dengan entity structure

**Contoh Code**:

```go
package request

// CreateProductRequest represents the request to create a product
type CreateProductRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Price       float64 `json:"price" binding:"required,gt=0"`
	Stock       int     `json:"stock" binding:"required,gte=0"`
	Category    string  `json:"category" binding:"required,oneof=electronics clothing food other"`
	Status      string  `json:"status" binding:"omitempty,oneof=active inactive draft archived"`
}

// UpdateProductRequest represents the request to update a product
type UpdateProductRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Price       *float64 `json:"price" binding:"omitempty,gt=0"`
	Stock       *int     `json:"stock" binding:"omitempty,gte=0"`
	Category    *string  `json:"category" binding:"omitempty,oneof=electronics clothing food other"`
	Status      *string  `json:"status" binding:"omitempty,oneof=active inactive draft archived"`
}
```

**Penjelasan**:
- **CreateProductRequest**: Struct untuk POST request (semua fields required kecuali optional)
- **UpdateProductRequest**: Struct untuk PUT request (semua fields optional, pakai pointers `*`)
- **Binding Tags**: Gin validation tags:
  - `required` - Field harus diisi
  - `gt=0` - Greater than 0
  - `gte=0` - Greater than or equal 0
  - `oneof=a b c` - Value harus salah satu dari opsi
  - `omitempty` - Tidak required jika kosong

**Mengapa UpdateRequest pakai Pointers?**
```go
// Tanpa pointer
type UpdateProductRequest struct {
	Name string `json:"name"`
}
// Problem: Tidak bisa bedakan antara "tidak diisi" vs "diisi dengan empty string"

// Dengan pointer
type UpdateProductRequest struct {
	Name *string `json:"name"`
}
// Solution:
// nil = tidak diisi (skip update field ini)
// "" = diisi dengan empty string (update field ke "")
// "Laptop" = diisi dengan "Laptop" (update field ke "Laptop")
```

---

## STEP 8: Buat Usecase Implementation 🎯

**File**: `internal/usecase/product/service.go`

**Apa itu Usecase Implementation?**
Ini adalah implementasi dari `domain/usecase/ProductUseCase` interface. Ini adalah tempat **business logic** berada.

**Perbedaan Repository vs Usecase**:

| Aspect | Repository | Usecase |
|--------|-----------|---------|
| Fokus | Data access (SQL) | Business logic (rules) |
| Menggunakan | GORM, SQL queries | Domain entities, validations |
| Flow | GORM → Database | Usecase → Repository → Database |
| Contoh | `SELECT * FROM products` | `if price <= 0 { return error }` |

**Contoh Code**:

```go
package productservice

import (
	"github.com/google/uuid"
	"github.com/rama/kit/internal/domain/entity"
	"github.com/rama/kit/internal/domain/repository"
	"github.com/rama/kit/internal/domain/usecase"
	apperrors "github.com/rama/kit/pkg/errors"
)

// Service implements ProductUseCase interface
type Service struct {
	productRepo repository.ProductRepository
}

// NewProductService creates a new product service
func NewProductService(productRepo repository.ProductRepository) usecase.ProductUseCase {
	return &Service{
		productRepo: productRepo,
	}
}

// CreateProduct creates a new product
func (s *Service) CreateProduct(creatorID, name, description string, price float64, stock int, category, status string) (*entity.Product, error) {
	// 1. Validate required fields
	if name == "" {
		return nil, apperrors.NewBadRequestError("name is required")
	}
	if price <= 0 {
		return nil, apperrors.NewBadRequestError("price must be greater than 0")
	}
	if stock < 0 {
		return nil, apperrors.NewBadRequestError("stock cannot be negative")
	}
	if !isValidCategory(category) {
		return nil, apperrors.NewBadRequestError("invalid category")
	}

	// 2. Set default status if not provided
	if status == "" {
		status = entity.ProductStatusActive
	}

	// 3. Validate status
	if !isValidStatus(status) {
		return nil, apperrors.NewBadRequestError("invalid status")
	}

	// 4. Create product entity
	product := &entity.Product{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
		Price:       price,
		Stock:       stock,
		Category:    category,
		Status:      status,
		CreatedBy:   creatorID,
		UpdatedBy:   creatorID,
	}

	// 5. Save via repository
	if err := s.productRepo.Create(product); err != nil {
		return nil, apperrors.Wrap(err, "failed to create product")
	}

	return product, nil
}

// GetProduct gets a product by ID
func (s *Service) GetProduct(id string) (*entity.Product, error) {
	// 1. Call repository
	product, err := s.productRepo.FindByID(id)
	if err != nil {
		return nil, apperrors.Wrap(err, "failed to get product")
	}

	// 2. Check if product exists
	if product == nil {
		return nil, apperrors.NewNotFoundError("product not found")
	}

	return product, nil
}

// ListProducts lists products with filters
func (s *Service) ListProducts(filter usecase.ProductFilterRequest) ([]*entity.Product, int64, error) {
	// 1. Validate pagination
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 {
		filter.Limit = 10
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	// 2. Validate category if provided
	if filter.Category != "" && !isValidCategory(filter.Category) {
		return nil, 0, apperrors.NewBadRequestError("invalid category")
	}

	// 3. Validate status if provided
	if filter.Status != "" && !isValidStatus(filter.Status) {
		return nil, 0, apperrors.NewBadRequestError("invalid status")
	}

	// 4. Call repository
	repoFilter := repository.ProductFilter{
		Category: filter.Category,
		Status:   filter.Status,
		Search:   filter.Search,
		Page:     filter.Page,
		Limit:    filter.Limit,
	}

	products, total, err := s.productRepo.FindAll(repoFilter)
	if err != nil {
		return nil, 0, apperrors.Wrap(err, "failed to list products")
	}

	return products, total, nil
}

// UpdateProduct updates a product
func (s *Service) UpdateProduct(id string, updaterID string, updates map[string]interface{}) (*entity.Product, error) {
	// 1. Get existing product
	product, err := s.productRepo.FindByID(id)
	if err != nil {
		return nil, apperrors.Wrap(err, "failed to get product")
	}
	if product == nil {
		return nil, apperrors.NewNotFoundError("product not found")
	}

	// 2. Apply updates
	if name, ok := updates["name"].(string); ok {
		if name == "" {
			return nil, apperrors.NewBadRequestError("name cannot be empty")
		}
		product.Name = name
	}
	if description, ok := updates["description"].(string); ok {
		product.Description = description
	}
	if price, ok := updates["price"].(float64); ok {
		if price <= 0 {
			return nil, apperrors.NewBadRequestError("price must be greater than 0")
		}
		product.Price = price
	}
	if stock, ok := updates["stock"].(int); ok {
		if stock < 0 {
			return nil, apperrors.NewBadRequestError("stock cannot be negative")
		}
		product.Stock = stock
	}
	if category, ok := updates["category"].(string); ok {
		if !isValidCategory(category) {
			return nil, apperrors.NewBadRequestError("invalid category")
		}
		product.Category = category
	}
	if status, ok := updates["status"].(string); ok {
		if !isValidStatus(status) {
			return nil, apperrors.NewBadRequestError("invalid status")
		}
		product.Status = status
	}

	// 3. Update updater
	product.UpdatedBy = updaterID

	// 4. Save via repository
	if err := s.productRepo.Update(product); err != nil {
		return nil, apperrors.Wrap(err, "failed to update product")
	}

	return product, nil
}

// DeleteProduct deletes a product
func (s *Service) DeleteProduct(id string) error {
	// 1. Check if product exists
	product, err := s.productRepo.FindByID(id)
	if err != nil {
		return apperrors.Wrap(err, "failed to get product")
	}
	if product == nil {
		return apperrors.NewNotFoundError("product not found")
	}

	// 2. Delete via repository
	if err := s.productRepo.Delete(id); err != nil {
		return apperrors.Wrap(err, "failed to delete product")
	}

	return nil
}

// GetProductStats gets product statistics
func (s *Service) GetProductStats() (*usecase.ProductStats, error) {
	total, _ := s.productRepo.CountByStatus("")
	active, _ := s.productRepo.CountByStatus(entity.ProductStatusActive)
	inactive, _ := s.productRepo.CountByStatus(entity.ProductStatusInactive)
	draft, _ := s.productRepo.CountByStatus(entity.ProductStatusDraft)
	archived, _ := s.productRepo.CountByStatus(entity.ProductStatusArchived)

	return &usecase.ProductStats{
		Total:    total,
		Active:   active,
		Inactive: inactive,
		Draft:    draft,
		Archived: archived,
	}, nil
}

// isValidCategory validates product category
func isValidCategory(category string) bool {
	validCategories := []string{
		entity.ProductCategoryElectronics,
		entity.ProductCategoryClothing,
		entity.ProductCategoryFood,
		entity.ProductCategoryOther,
	}
	for _, c := range validCategories {
		if category == c {
			return true
		}
	}
	return false
}

// isValidStatus validates product status
func isValidStatus(status string) bool {
	validStatuses := []string{
		entity.ProductStatusActive,
		entity.ProductStatusInactive,
		entity.ProductStatusDraft,
		entity.ProductStatusArchived,
	}
	for _, s := range validStatuses {
		if status == s {
			return true
		}
	}
	return false
}
```

**Penjelasan**:
- **Constructor**: `NewProductService()` mengambil repository dependency
- **Validations**: Business validations di sini (price > 0, stock >= 0, valid category/status)
- **Business Rules**: Set default values, apply updates, check existence
- **Error Handling**: Wrap errors dengan context message
- **Helper functions**: `isValidCategory()`, `isValidStatus()` untuk validation

**Error Types (pkg/errors)**:
- `NewBadRequestError()` - 400 Bad Request (invalid input)
- `NewNotFoundError()` - 404 Not Found (resource not found)
- `Wrap(err, "message")` - Wrap error dengan additional context

---

## STEP 9: Buat Handler 🎮

**File**: `internal/adapter/api/http/handler/product.go`

**Apa itu Handler?**
Handler (Controller) adalah layer yang:
- Menerima HTTP request
- Extract data dari request (JSON, query params, path params)
- Call usecase
- Return HTTP response

**Flow HTTP Request**:
```
HTTP Request → Middleware → Handler → Usecase → Repository → Database
    ↓
HTTP Response ← Handler ← Usecase ← Repository ← Database
```

**Contoh Code**:

```go
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rama/kit/internal/api/dto/request"
	"github.com/rama/kit/internal/domain/usecase"
	"github.com/rama/kit/pkg/response"
)

// ProductHandler handles product HTTP requests
type ProductHandler struct {
	productService usecase.ProductUseCase
}

// NewProductHandler creates a new product handler
func NewProductHandler(productService usecase.ProductUseCase) *ProductHandler {
	return &ProductHandler{
		productService: productService,
	}
}

// Create creates a new product
func (h *ProductHandler) Create(c *gin.Context) {
	// 1. Bind JSON to request struct
	var req request.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 2. Get user ID from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	// 3. Set default status if not provided
	status := req.Status
	if status == "" {
		status = "active"
	}

	// 4. Call usecase
	product, err := h.productService.CreateProduct(
		userID.(string),
		req.Name,
		req.Description,
		req.Price,
		req.Stock,
		req.Category,
		status,
	)
	if err != nil {
		response.Error(c, err)
		return
	}

	// 5. Return success response
	response.Success(c, product)
}

// GetByID gets a product by ID
func (h *ProductHandler) GetByID(c *gin.Context) {
	// 1. Extract ID from path parameter
	id := c.Param("id")

	// 2. Call usecase
	product, err := h.productService.GetProduct(id)
	if err != nil {
		response.Error(c, err)
		return
	}

	// 3. Return success response
	response.Success(c, product)
}

// List lists products with filters
func (h *ProductHandler) List(c *gin.Context) {
	// 1. Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	// 2. Build filter
	filter := usecase.ProductFilterRequest{
		Category: c.Query("category"),
		Status:   c.Query("status"),
		Search:   c.Query("search"),
		Page:     page,
		Limit:    limit,
	}

	// 3. Call usecase
	products, total, err := h.productService.ListProducts(filter)
	if err != nil {
		response.Error(c, err)
		return
	}

	// 4. Return success response with pagination metadata
	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"products": products,
			"total":    total,
			"page":     page,
			"limit":    limit,
		},
	})
}

// Update updates a product
func (h *ProductHandler) Update(c *gin.Context) {
	// 1. Extract ID from path parameter
	id := c.Param("id")

	// 2. Bind JSON to request struct
	var req request.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 3. Get user ID from context
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "user not authenticated")
		return
	}

	// 4. Build updates map
	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Price != nil {
		updates["price"] = *req.Price
	}
	if req.Stock != nil {
		updates["stock"] = *req.Stock
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}

	// 5. Check if at least one field is provided
	if len(updates) == 0 {
		response.BadRequest(c, "at least one field must be provided")
		return
	}

	// 6. Call usecase
	product, err := h.productService.UpdateProduct(id, userID.(string), updates)
	if err != nil {
		response.Error(c, err)
		return
	}

	// 7. Return success response
	response.Success(c, product)
}

// Delete deletes a product
func (h *ProductHandler) Delete(c *gin.Context) {
	// 1. Extract ID from path parameter
	id := c.Param("id")

	// 2. Call usecase
	if err := h.productService.DeleteProduct(id); err != nil {
		response.Error(c, err)
		return
	}

	// 3. Return success response
	response.Success(c, gin.H{
		"message": "product deleted successfully",
	})
}

// GetStats gets product statistics
func (h *ProductHandler) GetStats(c *gin.Context) {
	// 1. Call usecase
	stats, err := h.productService.GetProductStats()
	if err != nil {
		response.Error(c, err)
		return
	}

	// 2. Return success response
	response.Success(c, stats)
}
```

**Penjelasan**:
- **Constructor**: `NewProductHandler()` mengambil usecase dependency
- **`c.ShouldBindJSON(&req)`**: Bind JSON request body ke struct (otomatis validasi)
- **`c.Get("user_id")`**: Extract user ID dari context (dari auth middleware)
- **`c.Param("id")`**: Extract path parameter (URL: `/products/:id`)
- **`c.Query("key")`**: Extract query parameter (URL: `/products?page=1`)
- **`c.DefaultQuery("key", "default")`**: Extract dengan default value
- **`c.JSON(status, data)`**: Return JSON response
- **`response.Success()`**: Helper untuk success response
- **`response.Error()`**: Helper untuk error response
- **`response.BadRequest()`**: Helper untuk 400 error
- **`response.Unauthorized()`**: Helper untuk 401 error

---

## STEP 10: Update Router 🛣️

**File**: `internal/adapter/api/http/router/router.go`

**Apa itu Router?**
Router mendefinisikan URL paths dan menghubungkan ke handler yang sesuai.

**Routing Pattern**:
```
Method    Path                   Handler
--------  ---------------------  -----------------------
GET       /api/v1/products      productHandler.List
POST      /api/v1/products      productHandler.Create
GET       /api/v1/products/:id  productHandler.GetByID
PUT       /api/v1/products/:id  productHandler.Update
DELETE    /api/v1/products/:id  productHandler.Delete
```

**Contoh Code (update existing)**:

```go
// SetupRouter configures all routes and middleware
func SetupRouter(
	healthHandler *handler.HealthHandler,
	userHandler *handler.UserHandler,
	productHandler *handler.ProductHandler,  // ADD THIS PARAMETER
	authMiddleware *middleware.Auth,
	corsMiddleware *middleware.CORS,
	loggingMiddleware *middleware.Logger,
	rateLimitMiddleware *middleware.RateLimitMiddleware,
	requestIDMiddleware *middleware.RequestIDMiddleware,
	securityMiddleware *middleware.SecurityMiddleware,
) *gin.Engine {
	r := gin.New()

	// Global middleware
	r.Use(
		middleware.Recovery(),
		requestIDMiddleware.RequestID(),
		loggingMiddleware.Log(),
		corsMiddleware.Handle(),
		securityMiddleware.Security(),
		rateLimitMiddleware.RateLimit(),
	)

	// Public routes
	r.GET("/health", healthHandler.Health)

	// Protected routes (auth required)
	protected := r.Group("/api/v1")
	protected.Use(authMiddleware.Authenticate())
	{
		// User management routes
		users := protected.Group("/users")
		{
			users.POST("", userHandler.Create)
			users.GET("", userHandler.List)
			users.GET("/:id", userHandler.GetByID)
			users.PUT("/:id", userHandler.Update)
			users.DELETE("/:id", userHandler.Delete)
		}

		// Product management routes - ADD THIS
		products := protected.Group("/products")
		{
			products.POST("", productHandler.Create)
			products.GET("", productHandler.List)
			products.GET("/stats", productHandler.GetStats)
			products.GET("/:id", productHandler.GetByID)
			products.PUT("/:id", productHandler.Update)
			products.DELETE("/:id", productHandler.Delete)
		}
	}

	return r
}
```

**Penjelasan**:
- **Function Parameter**: Tambah `productHandler *handler.ProductHandler`
- **Route Group**: `protected.Group("/products")` membuat group untuk semua product routes
- **HTTP Methods**:
  - `POST` - Create resource
  - `GET` - Read resource
  - `PUT` - Update resource (full/partial update)
  - `DELETE` - Delete resource
- **Path Parameters**: `/:id` adalah dynamic parameter (di-access di handler dengan `c.Param("id")`)

---

## STEP 11: Update main.go 🚀

**File**: `cmd/server/main.go`

**Apa yang dilakukan di main.go?**
main.go adalah entry point aplikasi. Di sini kita:
- Load configuration
- Connect database
- Initialize semua komponen (repositories, usecases, handlers)
- Setup router
- Start server

**Dependency Injection**:
```
main.go
  ↓ Creates
Database
  ↓ Creates
ProductRepository
  ↓ Creates
ProductService (Usecase)
  ↓ Creates
ProductHandler
  ↓ Passed to
Router
```

**Contoh Code (update existing)**:

```go
package main

import (
	// ... existing imports ...

	// ADD THIS IMPORT
	productservice "github.com/rama/kit/internal/usecase/product"
)

func main() {
	// ... existing code (load config, connect DB, etc) ...

	// 1. Init repositories - ADD THIS
	userRepo := postgres.NewUserPersistence(db)
	productRepo := postgres.NewProductPersistence(db)

	// 2. Init usecases - ADD THIS
	var userService usecase.UserUseCase = userservice.NewUserService(userRepo)
	var productService usecase.ProductUseCase = productservice.NewProductService(productRepo)

	// 3. Init handlers - ADD THIS
	healthHandler := handler.NewHealthHandler()
	userHandler := handler.NewUserHandler(userService)
	productHandler := handler.NewProductHandler(productService)

	// 4. Init middleware (existing code, no change)
	// ...

	// 5. Setup router - UPDATE THIS
	r := router.SetupRouter(
		healthHandler,
		userHandler,
		productHandler,  // ADD THIS PARAMETER
		authMiddleware,
		corsMiddleware,
		loggingMiddleware,
		rateLimitMiddleware,
		requestIDMiddleware,
		securityMiddleware,
	)

	// ... rest of main.go (start server) ...
}
```

**Penjelasan**:
- **Import**: Tambah import untuk `productservice`
- **Repository Init**: Create `productRepo` dengan `NewProductPersistence(db)`
- **Usecase Init**: Create `productService` dengan `NewProductService(productRepo)`
- **Handler Init**: Create `productHandler` dengan `NewProductHandler(productService)`
- **Router Setup**: Pass `productHandler` ke `router.SetupRouter()`

**Order of Initialization (PENTING)**:
1. Connect Database
2. AutoMigrate (create tables)
3. Create Repositories (butuh DB connection)
4. Create Usecases (butuh Repositories)
5. Create Handlers (butuh Usecases)
6. Setup Router (butuh Handlers)
7. Start Server

---

## STEP 12: Build & Test ✅

**Command**: Build aplikasi

```bash
cd /Users/macbook/project/go/starter/kit
go build -o bin/server ./cmd/server
```

**Jika ada error**:
- Check import paths
- Check function signatures
- Check struct field names

**Command**: Test secara manual

```bash
# 1. Start server
./bin/server

# 2. Get JWT token from SSO
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@binawan.ac.id","password":"admin12345"}'

# Copy token from response

# 3. Test create product
curl -X POST http://localhost:8081/api/v1/products \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Laptop Gaming",
    "description": "High performance gaming laptop",
    "price": 15000000,
    "stock": 10,
    "category": "electronics",
    "status": "active"
  }'

# 4. Test list products
curl http://localhost:8081/api/v1/products?page=1&limit=10 \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 5. Test get product by ID
curl http://localhost:8081/api/v1/products/PRODUCT_ID \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 6. Test update product
curl -X PUT http://localhost:8081/api/v1/products/PRODUCT_ID \
  -H "Authorization: Bearer YOUR_JWT_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Laptop Gaming Updated",
    "price": 14000000
  }'

# 7. Test delete product
curl -X DELETE http://localhost:8081/api/v1/products/PRODUCT_ID \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"

# 8. Test get stats
curl http://localhost:8081/api/v1/products/stats \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

---

## 🎓 Summary: 12 Steps

| Step | File | Layer | Apa yang Dibuat? |
|------|------|-------|------------------|
| 1 | `internal/domain/entity/product.go` | Domain | Entity (PURE) |
| 2 | `internal/domain/repository/product_repository.go` | Domain | Repository Interface |
| 3 | `internal/domain/usecase/product_usecase.go` | Domain | Usecase Interface |
| 4 | `internal/repository/postgres/models.go` | Repository | GORM Model + Converters |
| 5 | `internal/repository/postgres/product_repository.go` | Repository | Repository Implementation |
| 6 | `internal/infrastructure/database/database.go` | Infrastructure | AutoMigrate |
| 7 | `internal/api/dto/request/product_request.go` | API | Request/Response DTOs |
| 8 | `internal/usecase/product/service.go` | Usecase | Business Logic |
| 9 | `internal/adapter/api/http/handler/product.go` | Adapter | HTTP Handler |
| 10 | `internal/adapter/api/http/router/router.go` | Adapter | Routes |
| 11 | `cmd/server/main.go` | Main | Dependency Injection |
| 12 | Terminal | - | Build & Test |

---

## 🔄 Data Flow Lengkap

### Create Product Flow

```
1. Client sends POST /api/v1/products
   ↓
2. Middleware Stack (Recovery, Request ID, Logging, CORS, Security, Rate Limit, Auth)
   ↓
3. Auth Middleware verifies JWT token
   - Extract user_id from token
   - Inject to context: c.Set("user_id", "user-uuid")
   ↓
4. ProductHandler.Create()
   - Bind JSON to CreateProductRequest
   - Extract user_id from context
   - Call productService.CreateProduct()
   ↓
5. ProductService.CreateProduct()
   - Validate inputs (name required, price > 0, etc)
   - Create Product entity
   - Call productRepo.Create(product)
   ↓
6. ProductRepository.Create()
   - Convert Product entity → ProductGORM model
   - db.Create(model)
   - GORM generates: INSERT INTO products ...
   ↓
7. PostgreSQL Database
   - Execute INSERT statement
   - Return success
   ↓
8. Response flows back (reverse)
   - Repository returns nil (success)
   - Usecase returns product entity
   - Handler returns JSON response
   - Middleware logs response
   - Client receives: { "success": true, "data": { ... } }
```

---

## 💡 Tips & Best Practices

### 1. **Naming Conventions**
- **Entities**: `Product`, `User`, `Order`
- **Interfaces**: `ProductRepository`, `ProductUseCase`
- **Implementations**: `productRepository` (struct), `Service` (struct)
- **Handlers**: `ProductHandler`
- **Files**: `product.go`, `product_repository.go`, `service.go`

### 2. **Error Handling**
- Use `pkg/errors` for consistent error types:
  - `NewBadRequestError()` - 400
  - `NewNotFoundError()` - 404
  - `NewConflictError()` - 409
  - `NewUnauthorizedError()` - 401
  - `NewForbiddenError()` - 403
  - `NewInternalError()` - 500
- Wrap errors with context: `apperrors.Wrap(err, "failed to create product")`

### 3. **Validation**
- **Handler Level**: Use Gin binding tags (`required`, `email`, `oneof`)
- **Usecase Level**: Business validations (price > 0, stock >= 0)
- **Database Level**: Use GORM tags (`not null`, `unique`)

### 4. **Pagination**
- Default: `page=1`, `limit=10`
- Max limit: `limit=100` (anti-DDoS)
- Return `total` count for UI pagination

### 5. **Soft Delete**
- Use GORM `DeletedAt` field
- Repository uses `db.Delete()` (soft delete)
- To hard delete: `db.Unscoped().Delete()`
- To include deleted: `db.Unscoped().Find()`

### 6. **Context Usage**
- `c.Set("key", value)` - Set value in context
- `c.Get("key")` - Get value from context
- Use for user_id, request_id, etc

### 7. **Pointer vs Value in Structs**
- **CreateRequest**: Use value (required fields)
- **UpdateRequest**: Use pointers (optional fields)
- **Why pointers?** To differentiate "not provided" vs "provided as empty"

---

## ❓ Common Mistakes & How to Fix

### Mistake 1: Forget to add GORM model to AutoMigrate
**Error**: Table not found
**Fix**: Add `&postgres.ProductGORM{}` to `AutoMigrate()`

### Mistake 2: Wrong import path
**Error**: cannot find package
**Fix**: Check `go.mod` module name, use correct path

### Mistake 3: Forget to convert Entity ↔ Model
**Error**: Type mismatch
**Fix**: Always convert between Entity and Model

### Mistake 4: Missing parameter in router.SetupRouter()
**Error**: compile error
**Fix**: Add all handler parameters in correct order

### Mistake 5: Not getting user_id from context
**Error: unauthorized**
**Fix**: Always check `c.Get("user_id")` in protected handlers

---

## 🎯 Checklist Before Deploying

- [ ] Build successful (`go build`)
- [ ] All tests passing (`make test`)
- [ ] AutoMigrate working (check DB tables)
- [ ] All API endpoints tested
- [ ] Error handling working
- [ ] Validation working
- [ ] Pagination working
- [ ] Authentication working
- [ ] Logging working
- [ ] Rate limiting working

---

## 📚 Further Reading

- **Clean Architecture**: https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html
- **Gin Framework**: https://gin-gonic.com/docs/
- **GORM Documentation**: https://gorm.io/docs/
- **Go Best Practices**: https://github.com/golang/go/wiki/CodeReviewComments

---

## 🎉 Conclusion

Selamat! Kamu sudah belajar cara menambah fitur baru di Starter Kit dengan Clean Architecture!

**Key Takeaways**:
1. **Follow the 12 steps** - Jangan skip!
2. **Keep layers separated** - Domain, Usecase, Repository, Adapter
3. **Use interfaces** - Dependency inversion principle
4. **Validate early** - Handler → Usecase → Repository
5. **Error handling** - Use consistent error types
6. **Test everything** - Build, manual testing, automated testing

**Next Steps**:
1. Coba buat fitur lain (Order, Category, dll)
2. Tambah unit tests
3. Tambah integration tests
4. Deploy to production

---

**Selamat berkarya, Mas Ram!** 🚀

Jika ada pertanyaan, jangan ragu untuk bertanya! 😊
