package response

import "time"

// ==================== SERVICE RESPONSES ====================

// Service is the public output for a service
type Service struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	BaseURL     string    `json:"base_url"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ==================== PERMISSION RESPONSES ====================

// Permission is the public output for a permission
type Permission struct {
	ID          string    `json:"id"`
	ServiceID   string    `json:"service_id"`
	Permission  string    `json:"permission"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// ==================== ACCESS RESPONSES ====================

// Access is the public output for a service access
type Access struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ServiceID string    `json:"service_id"`
	GrantedBy string    `json:"granted_by"`
	IsActive  bool      `json:"is_active"`
	GrantedAt time.Time `json:"granted_at"`
}

// ==================== USER PERMISSION RESPONSES ====================

// UserPermission is the public output for a user permission
type UserPermission struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Permission string    `json:"permission"`
	ServiceID  string    `json:"service_id"`
	GrantedBy  string    `json:"granted_by"`
	IsActive   bool      `json:"is_active"`
	GrantedAt  time.Time `json:"granted_at"`
}

// ==================== CHECK RESPONSES ====================

// AccessCheck is the output for access check
type AccessCheck struct {
	HasAccess bool   `json:"has_access"`
	UserID    string `json:"user_id"`
	ServiceID string `json:"service_id"`
}

// PermissionCheck is the output for permission check
type PermissionCheck struct {
	HasPermission bool   `json:"has_permission"`
	Permission    string `json:"permission"`
	ServiceID     string `json:"service_id"`
}

// ==================== MENU RESPONSES ====================

// MenuItem is one item in the user menu
type MenuItem struct {
	ServiceID   string   `json:"service_id"`
	ServiceName string   `json:"service_name"`
	BaseURL     string   `json:"base_url"`
	Permissions []string `json:"permissions"`
}

// ==================== COMMON ====================

// Pagination holds pagination metadata
type Pagination struct {
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}
