package entity

import (
	"time"

	"gorm.io/gorm"
)

// ==================== ACTION PLAN (F4 — pengendalian kinerja) ====================
// Pemicu: capaian di bawah threshold (kuning/merah). Blueprint F:
// template rencana perbaikan, PIC, deadline, progress, eskalasi.

type ActionPlanItem struct {
	Description string `json:"description"`
	PICName     string `json:"pic_name,omitempty"`
	DueDate     string `json:"due_date,omitempty"` // YYYY-MM-DD
	Status      string `json:"status"`             // pending|done
	CompletedAt string `json:"completed_at,omitempty"`
}

type ActionPlan struct {
	ID            string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	AchievementID string `json:"achievement_id" gorm:"type:varchar(36);not null;index"`
	IndicatorID   string `json:"indicator_id" gorm:"type:varchar(36);index"` // denormalisasi utk query cepat
	UnitID        string `json:"unit_id" gorm:"type:varchar(36);index"`
	PeriodID      string `json:"period_id" gorm:"type:varchar(36)"`

	ProblemStatement string           `json:"problem_statement" gorm:"type:text;not null"`
	Items            []ActionPlanItem `json:"items" gorm:"type:jsonb;serializer:json"`
	PICUserID        *string          `json:"pic_user_id,omitempty" gorm:"type:varchar(36)"` // HC user (opsional)
	Deadline         *time.Time       `json:"deadline,omitempty" gorm:"type:timestamptz;index"`
	ProgressPct      int              `json:"progress_pct" gorm:"default:0"`                       // dihitung dari items
	Status           string           `json:"status" gorm:"type:varchar(20);default:'open';index"` // open|done|overdue|escalated
	EscalatedAt      *time.Time       `json:"escalated_at,omitempty" gorm:"type:timestamptz"`
	EscalationNote   string           `json:"escalation_note,omitempty" gorm:"type:text"`
	CreatedBy        string           `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt        time.Time        `json:"created_at"`
	UpdatedAt        time.Time        `json:"updated_at"`
	DeletedAt        gorm.DeletedAt   `json:"deleted_at,omitempty" gorm:"index"`

	Achievement *AchievementRecord   `json:"achievement,omitempty" gorm:"foreignKey:AchievementID"`
	Indicator   *IndicatorDefinition `json:"indicator,omitempty" gorm:"foreignKey:IndicatorID"`
	Period      *Period              `json:"period,omitempty" gorm:"foreignKey:PeriodID"`
}

func (ActionPlan) TableName() string { return "action_plans" }

// ComputeProgress — % item selesai (auto saat update items).
func (ap *ActionPlan) ComputeProgress() {
	if ap.Items == nil || len(ap.Items) == 0 {
		return
	}
	done := 0
	for _, it := range ap.Items {
		if it.Status == "done" {
			done++
		}
	}
	ap.ProgressPct = done * 100 / len(ap.Items)
	if ap.ProgressPct == 100 {
		ap.Status = "done"
	}
}
