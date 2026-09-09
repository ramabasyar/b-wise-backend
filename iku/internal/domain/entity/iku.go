package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== REGULATORY VERSION (Compliance Engine — registry) ====================
// Setiap regulasi IKU (Kepmen) tersimpan sebagai versi. Blueprint v2 Bab 7:
// definisi & formula terikat versi regulasi; historis tidak di-overwrite.

type RegulatoryVersion struct {
	ID                 string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	RegulationCode     string         `json:"regulation_code" gorm:"type:varchar(100);uniqueIndex;not null"` // e.g. "Kepmendiktisaintek 358/M/KEP/2025"
	Title              string         `json:"title" gorm:"type:varchar(300);not null"`
	EffectiveDate      time.Time      `json:"effective_date" gorm:"type:timestamptz;not null"`
	DocumentURL        string         `json:"document_url,omitempty" gorm:"type:varchar(500)"`                  // link JDIH/dokumen resmi
	VerificationStatus string         `json:"verification_status" gorm:"type:varchar(16);default:'unverified'"` // unverified|verified (F7 JDIH)
	VerifiedAt         *time.Time     `json:"verified_at,omitempty" gorm:"type:timestamptz"`
	VerifiedBy         string         `json:"verified_by,omitempty" gorm:"type:varchar(100)"`
	Status             string         `json:"status" gorm:"type:varchar(20);default:'active';index"` // draft|active|superseded
	Notes              string         `json:"notes,omitempty" gorm:"type:text"`
	CreatedBy          string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
}

func (RegulatoryVersion) TableName() string { return "regulatory_versions" }

// ==================== INDICATOR DEFINITION ====================
// Master indikator (12 IKU nasional + IKT/KPI internal nanti).
// SATU-SATUNYA tempat definisi IKU hidup — sebagai DATA, bukan kode.

type IndicatorDefinition struct {
	ID           string         `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	IkuCode      string         `json:"iku_code" gorm:"type:varchar(30);index;not null"` // IKU-1 .. IKU-12 / IKT-xx / KPI-xx
	Name         string         `json:"name" gorm:"type:varchar(300);not null"`
	Description  string         `json:"description,omitempty" gorm:"type:text"`
	Nature       string         `json:"nature" gorm:"type:varchar(20);default:'wajib'"`                        // wajib|pilihan|partisipatif
	PeriodType   string         `json:"period_type" gorm:"type:varchar(20);default:'annual'"`                  // quarterly|semester|annual
	RollupRule   string         `json:"rollup_rule,omitempty" gorm:"type:varchar(20);default:'avg'"`           // sum|avg|last — agregasi nilai antar triwulan → tahunan (F9)
	Polarity     string         `json:"polarity,omitempty" gorm:"type:varchar(20);default:'higher_is_better'"` // higher_is_better|lower_is_better — arah "makin baik" utk perbandingan YoY (F10)
	Level        int            `json:"level" gorm:"default:0"`                                                // L0 nasional .. L6 individu (blueprint v2 §3.2)
	ApplicablePT string         `json:"applicable_pt,omitempty" gorm:"type:varchar(50)"`                       // all|ptn|ptn-bh|pts
	SortOrder    int            `json:"sort_order,omitempty" gorm:"default:0"`
	RegVersionID string         `json:"reg_version_id" gorm:"type:varchar(36);index;not null"`
	ValidFrom    *time.Time     `json:"valid_from,omitempty" gorm:"type:timestamptz"`
	ValidTo      *time.Time     `json:"valid_to,omitempty" gorm:"type:timestamptz"`
	IsActive     bool           `json:"is_active" gorm:"default:true;index"`
	CreatedBy    string         `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`

	RegVersion *RegulatoryVersion `json:"reg_version,omitempty" gorm:"foreignKey:RegVersionID"`
}

func (IndicatorDefinition) TableName() string { return "indicator_definitions" }

// ==================== PERIOD ====================
// Periode pengukuran — LIFECYCLE (F9):
// provisioned (tercipta otomatis, menunggu admin buka) → open (input aktif)
// → grace (due date lewat; input terlambat masih diterima) → closed (beku permanen;
// manual oleh reviewer/admin ATAU auto-close setelah grace). Reopen oleh admin (alasan wajib).

type Period struct {
	ID        string     `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Label     string     `json:"label" gorm:"type:varchar(50);uniqueIndex;not null"` // "TW1-2026", "S1-2026", "2026"
	Type      string     `json:"type" gorm:"type:varchar(20);not null"`              // quarterly|semester|annual
	Year      int        `json:"year" gorm:"not null;index"`
	Sequence  int        `json:"sequence,omitempty"` // triwulan ke-n / semester ke-n
	StartDate time.Time  `json:"start_date" gorm:"type:timestamptz"`
	EndDate   time.Time  `json:"end_date" gorm:"type:timestamptz"`
	DueDate   *time.Time `json:"due_date,omitempty" gorm:"type:timestamptz"`           // batas input/review (SLA-lite)
	GraceDays int        `json:"grace_days,omitempty" gorm:"default:14"`               // hari grace setelah due date → auto-close (F9)
	Status    string     `json:"status" gorm:"type:varchar(20);default:'provisioned'"` // provisioned|open|grace|closed
	OpenedBy  string     `json:"opened_by,omitempty" gorm:"type:varchar(100)"`
	OpenedAt  *time.Time `json:"opened_at,omitempty" gorm:"type:timestamptz"`
	ClosedBy  string     `json:"closed_by,omitempty" gorm:"type:varchar(100)"`
	ClosedAt  *time.Time `json:"closed_at,omitempty" gorm:"type:timestamptz"`
	CloseNote string     `json:"close_note,omitempty" gorm:"type:text"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (Period) TableName() string { return "periods" }

// GraceEnd — batas akhir input terlambat (due date + grace days). Setelah ini auto-close.
func (p Period) GraceEnd() time.Time {
	d := p.GraceDays
	if d <= 0 {
		d = 14
	}
	if p.DueDate == nil {
		return p.EndDate.AddDate(0, 0, d)
	}
	return p.DueDate.AddDate(0, 0, d)
}
