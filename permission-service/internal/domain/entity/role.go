package entity

import "time"

// Role represents a role template for a specific service
// Roles are service-scoped (e.g., "Admin Gudang" for Inventory Service)
type Role struct {
	ID          string     `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	Name        string     `json:"name" gorm:"type:varchar(100);not null"`
	ServiceID   string     `json:"service_id" gorm:"type:varchar(36);not null;index"`
	Description string     `json:"description" gorm:"type:text"`
	IsDefault   bool       `json:"is_default" gorm:"type:boolean;default:false"`
	CreatedBy   string     `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy   string     `json:"updated_by" gorm:"type:varchar(36)"`
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`

	// Relations
	Service    *Service          `json:"service,omitempty" gorm:"foreignKey:ServiceID"`
	Permissions []ServicePermission `json:"permissions,omitempty" gorm:"many2many:role_permissions;"`
}

func (Role) TableName() string {
	return "roles"
}

// UserRole represents the assignment of a role to a user
type UserRole struct {
	ID        string    `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	UserID    string    `json:"user_id" gorm:"type:varchar(36);not null;index"`
	RoleID    string    `json:"role_id" gorm:"type:varchar(36);not null;index"`
	GrantedBy string    `json:"granted_by" gorm:"type:varchar(36)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`

	// Relations
	Role *Role `json:"role,omitempty" gorm:"foreignKey:RoleID"`
}

func (UserRole) TableName() string {
	return "user_roles"
}
