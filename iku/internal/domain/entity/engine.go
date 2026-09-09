package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== FORMULA VERSION (Indicator Engine — F1) ====================
// Formula sebagai KONFIGURASI versioned (blueprint: Formula as Configuration).
// Ekspresi dievaluasi library expr-lang — tanpa akses jaringan/FS (sandbox).

type FormulaInputVar struct {
	Name       string `json:"name"`                  // nama variabel di ekspresi: count_kerja
	Label      string `json:"label"`                 // label di form: "Lulusan bekerja"
	Unit       string `json:"unit,omitempty"`        // "orang", "%"
	Type       string `json:"type,omitempty"`        // number (default) | percentage
	SourceHint string `json:"source_hint,omitempty"` // "Tracer Study / SIAKAD / manual"
	Required   bool   `json:"required,omitempty"`
}

type FormulaVersion struct {
	ID              string            `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	IndicatorID     string            `json:"indicator_id" gorm:"type:varchar(36);index;not null"`
	VersionNumber   int               `json:"version_number"`
	Expression      string            `json:"expression" gorm:"type:text;not null"` // e.g. (a+b+c)/total*100
	InputVariables  []FormulaInputVar `json:"input_variables" gorm:"type:jsonb;serializer:json"`
	RoundingRule    string            `json:"rounding_rule,omitempty" gorm:"type:varchar(20)"`      // 2dp|0dp|none
	ValidationRules string            `json:"validation_rules,omitempty" gorm:"type:jsonb"`         // {"min":0,"max":100,"warn_above":95}
	Status          string            `json:"status" gorm:"type:varchar(20);default:'draft';index"` // draft|active|retired
	ActiveFrom      *time.Time        `json:"active_from,omitempty" gorm:"type:timestamptz"`
	Notes           string            `json:"notes,omitempty" gorm:"type:text"`
	CreatedBy       string            `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	DeletedAt       gorm.DeletedAt    `json:"deleted_at,omitempty" gorm:"index"`
}

func (FormulaVersion) TableName() string { return "formula_versions" }

// ==================== THRESHOLD (status warna capaian) ====================

type IndicatorThreshold struct {
	ID          string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	IndicatorID string    `json:"indicator_id" gorm:"type:varchar(36);uniqueIndex;not null"`
	RedBelow    float64   `json:"red_below" gorm:"default:50"`    // < 50% = merah
	YellowBelow float64   `json:"yellow_below" gorm:"default:75"` // 50–75% = kuning
	GreenMin    float64   `json:"green_min" gorm:"default:75"`    // >= 75% = hijau
	UpdatedAt   time.Time `json:"updated_at"`
}

func (IndicatorThreshold) TableName() string { return "indicator_thresholds" }

// ==================== PK DOCUMENTS (PK-lite — F1) ====================

type PKDocument struct {
	ID        string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	UnitID    string         `json:"unit_id" gorm:"type:varchar(36);index;not null"` // HC branch/department ID
	UnitType  string         `json:"unit_type" gorm:"type:varchar(20);default:'branch'"`
	Year      int            `json:"year" gorm:"not null;index"` // tahun PK
	Title     string         `json:"title" gorm:"type:varchar(300);not null"`
	FileURL   string         `json:"file_url,omitempty" gorm:"type:varchar(500)"`
	Status    string         `json:"status" gorm:"type:varchar(20);default:'active'"` // draft|active|archived
	Notes     string         `json:"notes,omitempty" gorm:"type:text"`
	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (PKDocument) TableName() string { return "pk_documents" }

// ==================== PERFORMANCE TARGET (Target Engine — F1) ====================

type PerformanceTarget struct {
	ID              string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	IndicatorID     string         `json:"indicator_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_tgt"`
	UnitID          string         `json:"unit_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_tgt"` // "institution" untuk level institusi
	UnitType        string         `json:"unit_type" gorm:"type:varchar(20);default:'institution'"`      // institution|branch|department
	PeriodID        string         `json:"period_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_tgt"`
	TargetValue     float64        `json:"target_value"`
	AggregationRule string         `json:"aggregation_rule,omitempty" gorm:"type:varchar(20)"` // sum|avg|custom (agregasi target sub-unit)
	PKRefID         *string        `json:"pk_ref_id,omitempty" gorm:"type:varchar(36);index"`
	SetBy           string         `json:"set_by,omitempty" gorm:"type:varchar(36)"`
	ApprovedBy      *string        `json:"approved_by,omitempty" gorm:"type:varchar(36)"`
	Status          string         `json:"status" gorm:"type:varchar(20);default:'draft'"` // draft|approved
	Notes           string         `json:"notes,omitempty" gorm:"type:text"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	Indicator *IndicatorDefinition `json:"indicator,omitempty" gorm:"foreignKey:IndicatorID"`
	Period    *Period              `json:"period,omitempty" gorm:"foreignKey:PeriodID"`
}

func (PerformanceTarget) TableName() string { return "performance_targets" }
