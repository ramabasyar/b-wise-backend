package request

// ==================== SERVICE REQUESTS ====================

// CreateService is the input for registering a new service
type CreateService struct {
	Name        string `json:"name" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"omitempty,max=500"`
	BaseURL     string `json:"base_url" binding:"required,url"`
}

// UpdateService is the input for updating a service
type UpdateService struct {
	Name        *string `json:"name" binding:"omitempty,min=1,max=100"`
	Description *string `json:"description" binding:"omitempty,max=500"`
	BaseURL     *string `json:"base_url" binding:"omitempty,url"`
}

// ListServices is the query parameters for listing services
type ListServices struct {
	Page     int `form:"page" binding:"omitempty,min=1"`
	PageSize int `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// ==================== PERMISSION REQUESTS ====================

// CreatePermission is the input for creating a permission
type CreatePermission struct {
	Permission  string `json:"permission" binding:"required,min=1,max=100"`
	Description string `json:"description" binding:"omitempty,max=500"`
}

// SeedPermissions is the input for seeding multiple permissions
type SeedPermissions struct {
	Permissions []string `json:"permissions" binding:"required,min=1"`
}

// ==================== ACCESS REQUESTS ====================

// GrantAccess is the input for granting service access
type GrantAccess struct {
	UserID string `json:"user_id" binding:"required"`
}

// ==================== USER PERMISSION REQUESTS ====================

// GrantUserPermission is the input for granting a user permission
type GrantUserPermission struct {
	Permission string `json:"permission" binding:"required,min=1,max=100"`
	ServiceID  string `json:"service_id" binding:"required"`
}
