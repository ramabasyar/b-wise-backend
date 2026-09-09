package entity

import "time"

// Service represents a registered microservice
type Service struct {
	ID          string    `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	Name        string    `json:"name" gorm:"type:varchar(100);uniqueIndex;not null"`
	Description string    `json:"description" gorm:"type:text"`
	BaseURL     string    `json:"base_url" gorm:"type:varchar(255);not null"` // e.g., http://localhost:8081
	IsActive    bool      `json:"is_active" gorm:"type:boolean;default:true"`
	CreatedBy   string    `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy   string    `json:"updated_by" gorm:"type:varchar(36)"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty" gorm:"index"`
}

// TableName specifies the table name
func (Service) TableName() string {
	return "services"
}