package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== DATA CONNECTOR (F5 — Data & Integration Broker) ====================
// Contract-first: setiap sumber data dideklarasikan dulu (field/format/frekuensi),
// baru diimplementasikan. Tipe: manual (import terkelola) | api | db | file-import.
// Karena SIAKAD/LMS vendor tidak open API → jalur utama = file-import (Excel/CSV).

type ConnectorType string

const (
	ConnectorManual     ConnectorType = "manual"      // form manual (F2, sudah jalan)
	ConnectorFileImport ConnectorType = "file-import" // Excel/CSV terkelola — F5 core
	ConnectorAPI        ConnectorType = "api"         // REST (moodle/HC nanti)
	ConnectorDB         ConnectorType = "db"          // read-view (opsional)
)

// ContractField — satu field kontrak data.
type ContractField struct {
	Name     string `json:"name"`               // nama kolom sumber: "jumlah_lulus_kerja"
	Target   string `json:"target"`             // variabel formula / kolom tujuan: "count_kerja"
	Type     string `json:"type,omitempty"`     // number|string|date
	Required bool   `json:"required,omitempty"` // wajib ada di file
}

type DataSourceConnector struct {
	ID           string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Name         string         `json:"name" gorm:"type:varchar(100);not null"`     // "Tracer Study (import)"
	IndicatorID  string         `json:"indicator_id" gorm:"type:varchar(36);index"` // opsional: connector per indikator
	Type         ConnectorType  `json:"type" gorm:"type:varchar(20);not null"`
	SystemOwner  string         `json:"system_owner,omitempty" gorm:"type:varchar(100)"` // "Alumni/SDK" — pemilik data
	ContractJSON string         `json:"contract,omitempty" gorm:"type:jsonb"`            // contract_fields + deskripsi
	Status       string         `json:"status" gorm:"type:varchar(20);default:'active'"` // active|inactive
	LastSyncAt   *time.Time     `json:"last_sync_at,omitempty" gorm:"type:timestamptz"`
	LastRunInfo  string         `json:"last_run_info,omitempty" gorm:"type:text"` // hasil run terakhir
	Notes        string         `json:"notes,omitempty" gorm:"type:text"`
	CreatedBy    string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (DataSourceConnector) TableName() string { return "data_source_connectors" }

// ==================== IMPORT BATCH (audit terkelola) ====================

type ImportBatch struct {
	ID            string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	ConnectorID   string         `json:"connector_id" gorm:"type:varchar(36);index;not null"`
	IndicatorID   string         `json:"indicator_id" gorm:"type:varchar(36);index"`
	UnitID        string         `json:"unit_id" gorm:"type:varchar(36)"` // unit target capaian
	PeriodID      string         `json:"period_id" gorm:"type:varchar(36)"`
	FileName      string         `json:"file_name" gorm:"type:varchar(255)"`
	TotalRows     int            `json:"total_rows"`
	ValidRows     int            `json:"valid_rows"`
	ErrorRows     int            `json:"error_rows"`
	Applied       bool           `json:"applied" gorm:"default:false"`                           // false = preview saja
	AchievementID *string        `json:"achievement_id,omitempty" gorm:"type:varchar(36);index"` // capaian yang terbentuk saat apply
	RawPreview    string         `json:"raw_preview,omitempty" gorm:"type:jsonb"`                // baris + error utk UI preview
	ImportedBy    string         `json:"imported_by,omitempty" gorm:"type:varchar(36)"`
	ImportedAt    time.Time      `json:"imported_at"`
	DeletedAt     gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (ImportBatch) TableName() string { return "import_batches" }
