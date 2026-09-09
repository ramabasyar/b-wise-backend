package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== REGULATION CHANGE WORKFLOW (F6 — Compliance Engine) ====================
// Blueprint Bab 7.3: draft regulasi baru → identifikasi IKU terdampak → draft formula
// → Impact Analyzer (simulasi, tanpa menyimpan) → review → approval → aktivasi.
// Prinsip: data historis TIDAK di-backfill; formula baru berlaku ke depan.

type RegulationChange struct {
	ID            string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Title         string         `json:"title" gorm:"type:varchar(300);not null"` // "Regulasi IKU 2027 (draft)"
	NewRegCode    string         `json:"new_reg_code" gorm:"type:varchar(100)"`   // kode regulasi baru
	NewRegTitle   string         `json:"new_reg_title" gorm:"type:varchar(300)"`
	EffectiveDate time.Time      `json:"effective_date" gorm:"type:timestamptz"`               // tanggal formula baru berlaku
	Status        string         `json:"status" gorm:"type:varchar(20);default:'draft';index"` // draft|in_review|approved|rejected|active
	ReviewNotes   string         `json:"review_notes,omitempty" gorm:"type:text"`
	RequestedBy   string         `json:"requested_by,omitempty" gorm:"type:varchar(36)"`
	ReviewedBy    *string        `json:"reviewed_by,omitempty" gorm:"type:varchar(36)"`
	ReviewedAt    *time.Time     `json:"reviewed_at,omitempty" gorm:"type:timestamptz"`
	ApprovedBy    *string        `json:"approved_by,omitempty" gorm:"type:varchar(36)"`
	ApprovedAt    *time.Time     `json:"approved_at,omitempty" gorm:"type:timestamptz"`
	ActivatedAt   *time.Time     `json:"activated_at,omitempty" gorm:"type:timestamptz"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	Items []RegulationChangeItem `json:"items" gorm:"foreignKey:ChangeID"`
}

func (RegulationChange) TableName() string { return "regulation_changes" }

type RegulationChangeItem struct {
	ID                string            `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	ChangeID          string            `json:"change_id" gorm:"type:varchar(36);index;not null"`
	IndicatorID       string            `json:"indicator_id" gorm:"type:varchar(36);not null"`
	ChangeType        string            `json:"change_type" gorm:"type:varchar(30);default:'formula_update'"` // formula_update|definition_update
	NewExpression     string            `json:"new_expression,omitempty" gorm:"type:text"`                    // draft formula baru
	NewInputVariables []FormulaInputVar `json:"new_input_variables,omitempty" gorm:"type:jsonb;serializer:json"`
	Notes             string            `json:"notes,omitempty" gorm:"type:text"`
}

func (RegulationChangeItem) TableName() string { return "regulation_change_items" }
