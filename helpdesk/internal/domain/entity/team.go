package entity

import "time"

// Team — cermin unit Human Capital (keputusan D-15, blueprint Pusat Layanan v1.1).
// Dibuat dan dinonaktifkan OTOMATIS oleh job sinkronisasi (internal/service/teamsync):
// setiap unit HC aktif non-group menjadi tim yang melayani dan mengirim tiket.
// Field operasional (EmailAlias) dikelola admin unit dan tidak disentuh sinkronisasi.
type Team struct {
	Base
	HCDepartmentID string    `json:"hc_department_id" gorm:"type:varchar(36);uniqueIndex;not null"`
	Code           string    `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"`
	Name           string    `json:"name" gorm:"type:varchar(200);not null"`
	OrgType        string    `json:"org_type,omitempty" gorm:"type:varchar(30)"`
	IsFrontOffice  bool      `json:"is_front_office" gorm:"default:false"`
	EmailAlias     string    `json:"email_alias,omitempty" gorm:"type:varchar(255)"`
	IsActive       bool      `json:"is_active" gorm:"default:true;index"`
	SyncedAt       time.Time `json:"synced_at"`
}

func (Team) TableName() string { return "teams" }
