package entity

import "time"

// UserPermission represents a detailed permission granted to a user
type UserPermission struct {
	ID          string    `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	UserID      string    `json:"user_id" gorm:"type:varchar(36);not null;index:idx_user_perm"`
	Permission  string    `json:"permission" gorm:"type:varchar(100);not null"` // e.g., employees.read, employees.write
	ServiceID   string    `json:"service_id" gorm:"type:varchar(36);not null;index:idx_user_perm"` // Optional: for permission isolation
	GrantedAt   time.Time `json:"granted_at" gorm:"default:now()"`
	GrantedBy   string    `json:"granted_by" gorm:"type:varchar(36)"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	RevokedBy   string    `json:"revoked_by,omitempty"`
	IsActive    bool      `json:"is_active" gorm:"default:true;index"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName specifies the table name
func (UserPermission) TableName() string {
	return "user_permissions"
}