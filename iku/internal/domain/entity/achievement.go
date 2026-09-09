package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== ACHIEVEMENT RECORD (F2 — inti) ====================
// Raw data (immutable setelah submit) → kalkulasi formula versi periode tsb
// → workflow draft→submitted→reviewed→approved→published (RACI blueprint).

type AchievementRecord struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	IndicatorID string `json:"indicator_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_ach"`
	UnitID      string `json:"unit_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_ach"` // institution|branch-id|dept-id
	UnitType    string `json:"unit_type" gorm:"type:varchar(20);default:'institution'"`
	PeriodID    string `json:"period_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_ach"`

	RawData         map[string]float64 `json:"raw_data" gorm:"type:jsonb;serializer:json"`         // input variabel formula
	Source          string             `json:"source" gorm:"type:varchar(20);default:'manual'"`    // manual|import|connector|rollup
	FormulaID       *string            `json:"formula_id,omitempty" gorm:"type:varchar(36);index"` // versi formula saat kalkulasi
	CalculatedValue float64            `json:"calculated_value"`
	AchievementPct  *float64           `json:"achievement_pct,omitempty"`                         // vs target (diisi saat ada target)
	TargetValue     *float64           `json:"target_value,omitempty"`                            // snapshot target saat kalkulasi
	ThresholdColor  string             `json:"threshold_color,omitempty" gorm:"type:varchar(10)"` // red|yellow|green

	Status      string     `json:"status" gorm:"type:varchar(20);default:'draft';index"` // draft|submitted|reviewed|approved|published|rejected|missed
	Stale       bool       `json:"stale" gorm:"default:false"`                           // rollup tahunan kedaluwarsa (TW di-reopen pasca generate) - perlu regenerate
	ReviewNotes string     `json:"review_notes,omitempty" gorm:"type:text"`
	SubmittedBy *string    `json:"submitted_by,omitempty" gorm:"type:varchar(36)"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty" gorm:"type:timestamptz"`
	ReviewedBy  *string    `json:"reviewed_by,omitempty" gorm:"type:varchar(36)"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty" gorm:"type:timestamptz"`
	ApprovedBy  *string    `json:"approved_by,omitempty" gorm:"type:varchar(36)"`
	ApprovedAt  *time.Time `json:"approved_at,omitempty" gorm:"type:timestamptz"`
	PublishedAt *time.Time `json:"published_at,omitempty" gorm:"type:timestamptz"`

	CreatedBy string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	Indicator *IndicatorDefinition `json:"indicator,omitempty" gorm:"foreignKey:IndicatorID"`
	Period    *Period              `json:"period,omitempty" gorm:"foreignKey:PeriodID"`
	Evidences []EvidenceDocument   `json:"evidences,omitempty" gorm:"foreignKey:AchievementID"`
}

func (AchievementRecord) TableName() string { return "achievement_records" }

// ==================== EVIDENCE (F2) ====================

type EvidenceDocument struct {
	ID            string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	AchievementID string         `json:"achievement_id" gorm:"type:varchar(36);not null;index"`
	FileName      string         `json:"file_name" gorm:"type:varchar(255);not null"`
	FileType      string         `json:"file_type,omitempty" gorm:"type:varchar(100)"`
	FileSize      int64          `json:"file_size,omitempty"`
	StoragePath   string         `json:"storage_path" gorm:"type:varchar(500);not null"`         // key di storage (S3/minio atau path lokal)
	StorageDriver string         `json:"storage_driver" gorm:"type:varchar(20);default:'local'"` // s3|local
	Metadata      string         `json:"metadata,omitempty" gorm:"type:jsonb"`                   // notes, checksum, dsb
	UploadedBy    string         `json:"uploaded_by,omitempty" gorm:"type:varchar(36)"`
	UploadedAt    time.Time      `json:"uploaded_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (EvidenceDocument) TableName() string { return "evidence_documents" }

// ==================== WORKFLOW LOG (audit per transisi — F2) ====================

type WorkflowLog struct {
	ID            string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	AchievementID string    `json:"achievement_id" gorm:"type:varchar(36);not null;index"`
	Action        string    `json:"action" gorm:"type:varchar(30);not null"` // created|recalculated|submitted|reviewed|approved|published|rejected|evidence_added
	ActorID       string    `json:"actor_id,omitempty" gorm:"type:varchar(36)"`
	ActorRole     string    `json:"actor_role,omitempty" gorm:"type:varchar(50)"` // dari permission scope
	Notes         string    `json:"notes,omitempty" gorm:"type:text"`
	IPAddress     string    `json:"ip_address,omitempty" gorm:"type:varchar(45)"`
	CreatedAt     time.Time `json:"created_at"`
}

func (WorkflowLog) TableName() string { return "workflow_logs" }
