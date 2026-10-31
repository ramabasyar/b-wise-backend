package entity

import "time"

// ServiceCategory — kategori tampilan katalog (lintas tim; blueprint §5.4).
type ServiceCategory struct {
	Base
	Name string `json:"name" gorm:"type:varchar(100);not null"`
	Icon string `json:"icon,omitempty" gorm:"type:varchar(50)"`
	Sort int    `json:"sort" gorm:"default:0"`
}

func (ServiceCategory) TableName() string { return "service_categories" }

// Service — layanan katalog milik satu tim; standar pelayanan ikut disimpan (blueprint §5.4).
type Service struct {
	Base
	TeamID          string `json:"team_id" gorm:"type:varchar(36);index;not null"`
	CategoryID      string `json:"category_id,omitempty" gorm:"type:varchar(36);index"`
	Slug            string `json:"slug" gorm:"type:varchar(120);uniqueIndex;not null"`
	Name            string `json:"name" gorm:"type:varchar(200);not null"`
	Description     string `json:"description,omitempty" gorm:"type:text"`
	TicketType      string `json:"ticket_type" gorm:"type:varchar(20);default:'request'"`
	Audience        string `json:"audience,omitempty" gorm:"type:jsonb"`
	Requirements    string `json:"requirements,omitempty" gorm:"type:text"`
	Procedure       string `json:"procedure,omitempty" gorm:"type:text"`
	FeeNote         string `json:"fee_note,omitempty" gorm:"type:text"`
	OutputNote      string `json:"output_note,omitempty" gorm:"type:text"`
	SLAPolicyID     string `json:"sla_policy_id,omitempty" gorm:"type:varchar(36)"`
	ApprovalFlow    string `json:"approval_flow,omitempty" gorm:"type:jsonb"`
	TaskTemplate    string `json:"task_template,omitempty" gorm:"type:jsonb"`
	Confidentiality string `json:"confidentiality" gorm:"type:varchar(20);default:'normal'"`
	AutoCloseDays   int    `json:"auto_close_days,omitempty" gorm:"default:0"`
	Status          string `json:"status" gorm:"type:varchar(20);default:'draft';index"`
}

func (Service) TableName() string { return "services" }

// ServiceForm — formulir dinamis per layanan; versi terbit tidak boleh diubah (KT-06).
type ServiceForm struct {
	Base
	ServiceID   string     `json:"service_id" gorm:"type:varchar(36);index;not null"`
	Version     int        `json:"version" gorm:"not null"`
	Schema      string     `json:"schema" gorm:"type:jsonb"`
	PublishedAt *time.Time `json:"published_at,omitempty" gorm:"type:timestamptz"`
}

func (ServiceForm) TableName() string { return "service_forms" }
