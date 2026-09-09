package entity

import "time"

// MenuItem merepresentasikan item navigasi di portal (sidebar B-Wise).
// Menu dimiliki oleh service (microservice mendaftarkan menu-nya sendiri),
// bisa bertingkat (parent_id), dan tampil hanya jika user punya
// required_permission (kosong = cukup punya akses service).
type MenuItem struct {
	ID                 string     `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	ServiceID          string     `json:"service_id" gorm:"type:varchar(36);not null;index"`
	ParentID           *string    `json:"parent_id,omitempty" gorm:"type:varchar(36);index"` // nil = root menu service ini
	Label              string     `json:"label" gorm:"type:varchar(100);not null"`          // e.g. "Employees"
	Path               string     `json:"path" gorm:"type:varchar(255)"`                    // route frontend, e.g. "/dashboard/human-capital/employees"
	Icon               string     `json:"icon" gorm:"type:varchar(60)"`                     // nama ikon (lucide), frontend mapping
	SortOrder          int        `json:"sort_order" gorm:"default:0"`
	RequiredPermission string     `json:"required_permission,omitempty" gorm:"type:varchar(100)"` // e.g. employees.read; kosong = semua yang punya akses
	IsActive           bool       `json:"is_active" gorm:"default:true;index"`
	CreatedBy          string     `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedAt          time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
	CreatedAt          time.Time  `json:"created_at" gorm:"autoCreateTime"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty" gorm:"index"`

	// Relations
	Children []MenuItem `json:"children,omitempty" gorm:"foreignKey:ParentID"`
}

// TableName specifies the table name
func (MenuItem) TableName() string {
	return "menu_items"
}
