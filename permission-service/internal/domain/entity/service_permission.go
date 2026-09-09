package entity

import "time"

// ServicePermission represents a permission definition for a service
type ServicePermission struct {
	ID          string `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	ServiceID   string `json:"service_id" gorm:"type:varchar(36);not null;index"`
	Permission  string `json:"permission" gorm:"type:varchar(100);not null"` // e.g., employees.read, employees.write
	Description string `json:"description" gorm:"type:text"`
	IsActive    bool   `json:"is_active" gorm:"default:true;index"` // false = dinonaktifkan (self-sync), bukan terhapus
	CreatedBy   string `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy   string `json:"updated_by" gorm:"type:varchar(36)"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName specifies the table name
func (ServicePermission) TableName() string {
	return "service_permissions"
}