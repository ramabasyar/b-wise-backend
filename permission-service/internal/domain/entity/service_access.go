package entity

import "time"

// ServiceAccess represents user access to a service
type ServiceAccess struct {
	ID           string    `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	UserID       string    `json:"user_id" gorm:"type:varchar(36);not null;index:idx_user_service"`
	ServiceID    string    `json:"service_id" gorm:"type:varchar(36);not null;index:idx_user_service"`
	GrantedAt    time.Time `json:"granted_at" gorm:"default:now()"`
	GrantedBy    string    `json:"granted_by" gorm:"type:varchar(36)"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokedBy    string    `json:"revoked_by,omitempty"`
	IsActive     bool      `json:"is_active" gorm:"default:true;index"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName specifies the table name
func (ServiceAccess) TableName() string {
	return "service_access"
}