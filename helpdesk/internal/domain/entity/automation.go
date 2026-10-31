package entity

// AutomationRule - aturan otomatisasi tim (KT-10/OT-05): jika [kejadian] dan
// [kondisi] maka [aksi]. Trigger tiket_dibuat / tiket_selesai.
type AutomationRule struct {
	Base
	TeamID       string `json:"team_id" gorm:"type:varchar(36);index;not null"`
	Name         string `json:"name" gorm:"type:varchar(150)"`
	TriggerEvent string `json:"trigger" gorm:"type:varchar(30);default:'tiket_dibuat'"`
	Conditions   string `json:"conditions" gorm:"type:jsonb"`
	Actions      string `json:"actions" gorm:"type:jsonb"`
	IsActive     bool   `json:"is_active" gorm:"default:true"`
}

func (AutomationRule) TableName() string { return "automation_rules" }
