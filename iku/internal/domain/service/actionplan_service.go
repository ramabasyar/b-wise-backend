package service

import (
	"errors"
	"fmt"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== ACTION PLAN SERVICE (F4) ====================

type ActionPlanService struct {
	db       *gorm.DB
	Notifier *Notifier // F7: optional
}

func NewActionPlanService(db *gorm.DB) *ActionPlanService { return &ActionPlanService{db: db} }

type ActionPlanFilter struct {
	AchievementID string
	IndicatorID   string
	UnitID        string
	Status        string
	OnlyOverdue   bool
}

func (s *ActionPlanService) List(f ActionPlanFilter) ([]entity.ActionPlan, error) {
	q := s.db.Model(&entity.ActionPlan{})
	if f.AchievementID != "" {
		q = q.Where("achievement_id = ?", f.AchievementID)
	}
	if f.IndicatorID != "" {
		q = q.Where("indicator_id = ?", f.IndicatorID)
	}
	if f.UnitID != "" {
		q = q.Where("unit_id = ?", f.UnitID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.OnlyOverdue {
		q = q.Where("deadline IS NOT NULL AND deadline < ? AND status <> 'done'", time.Now())
	}
	var list []entity.ActionPlan
	return list, q.Preload("Indicator").Preload("Period").Preload("Achievement").
		Order("created_at DESC").Limit(300).Find(&list).Error
}

func (s *ActionPlanService) Get(id string) (*entity.ActionPlan, error) {
	var ap entity.ActionPlan
	if err := s.db.Preload("Indicator").Preload("Period").Preload("Achievement").
		Where("id = ?", id).First(&ap).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ap, nil
}

type CreateActionPlanInput struct {
	AchievementID    string
	ProblemStatement string
	Items            []entity.ActionPlanItem
	PICUserID        *string
	Deadline         *time.Time
}

// Create — action plan menempel pada capaian (satu aktif per capaian).
func (s *ActionPlanService) Create(in CreateActionPlanInput, actor string) (*entity.ActionPlan, error) {
	if in.AchievementID == "" || in.ProblemStatement == "" || len(in.Items) == 0 {
		return nil, fmt.Errorf("%w: capaian, pernyataan masalah, dan minimal 1 item wajib", ErrValidation)
	}
	var a entity.AchievementRecord
	if err := s.db.Where("id = ?", in.AchievementID).First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	// cek plan aktif existing
	var openCount int64
	s.db.Model(&entity.ActionPlan{}).
		Where("achievement_id = ? AND status <> 'done'", in.AchievementID).Count(&openCount)
	if openCount > 0 {
		return nil, fmt.Errorf("%w: sudah ada action plan aktif untuk capaian ini", ErrValidation)
	}

	ap := &entity.ActionPlan{
		AchievementID: in.AchievementID, IndicatorID: a.IndicatorID,
		UnitID: a.UnitID, PeriodID: a.PeriodID,
		ProblemStatement: in.ProblemStatement, Items: in.Items,
		PICUserID: in.PICUserID, Deadline: in.Deadline,
		Status: "open", CreatedBy: actor,
	}
	ap.ComputeProgress()
	if err := s.db.Create(ap).Error; err != nil {
		return nil, err
	}
	return ap, nil
}

// UpdateItems — ganti daftar item (progress dihitung ulang).
func (s *ActionPlanService) UpdateItems(id string, items []entity.ActionPlanItem, deadline *time.Time) (*entity.ActionPlan, error) {
	var ap entity.ActionPlan
	if err := s.db.Where("id = ?", id).First(&ap).Error; err != nil {
		return nil, ErrNotFound
	}
	if ap.Status == "done" {
		return nil, fmt.Errorf("%w: action plan sudah selesai", ErrValidation)
	}
	ap.Items = items
	if deadline != nil {
		ap.Deadline = deadline
	}
	ap.ComputeProgress()
	if err := s.db.Save(&ap).Error; err != nil {
		return nil, err
	}
	return &ap, nil
}

// Escalate — tandai eskalasi (dipanggil manual atau job scheduler saat overdue).
func (s *ActionPlanService) Escalate(id, note string) (*entity.ActionPlan, error) {
	var ap entity.ActionPlan
	if err := s.db.Where("id = ?", id).First(&ap).Error; err != nil {
		return nil, ErrNotFound
	}
	if ap.Status == "done" {
		return nil, fmt.Errorf("%w: action plan sudah selesai", ErrValidation)
	}
	now := time.Now()
	ap.Status = "escalated"
	ap.EscalatedAt = &now
	if note != "" {
		ap.EscalationNote = note
	}
	if err := s.db.Save(&ap).Error; err != nil {
		return nil, err
	}
	return &ap, nil
}

// EscalateOverdue — batch: tandai semua open yang lewat deadline. Return jumlah.
// Dipanggil saat boot + bisa dipanggil scheduler berkala.
func (s *ActionPlanService) EscalateOverdue() (int64, error) {
	var due []entity.ActionPlan
	if err := s.db.
		Where("deadline IS NOT NULL AND deadline < ? AND status = 'open'", time.Now()).
		Find(&due).Error; err != nil {
		return 0, err
	}
	now := time.Now()
	for i := range due {
		if err := s.db.Model(&entity.ActionPlan{}).Where("id = ?", due[i].ID).
			Updates(map[string]interface{}{
				"status": "escalated", "escalated_at": now,
				"escalation_note": "Otomatis: melewati tenggat tanpa selesai",
			}).Error; err != nil {
			return int64(i), err
		}
		// F7: eskalasi notifikasi ke PIC
		if s.Notifier != nil && due[i].PICUserID != nil && *due[i].PICUserID != "" {
			s.Notifier.Notify([]string{*due[i].PICUserID}, "actionplan.overdue",
				"Action plan melewati tenggat",
				fmt.Sprintf("Action plan \"%s\" melewati tenggat %s dan kini berstatus ESCALATED. Segera tindak lanjut atau minta penyesuaian.", due[i].ProblemStatement, due[i].Deadline.Format("02-01-2006")),
				"action_plan", due[i].ID)
		}
	}
	return int64(len(due)), nil
}

// Suggestions — capaian di bawah threshold yang belum punya action plan aktif
// (pemicu otomatis blueprint — kita tampilkan sebagai SARAN, bukan auto-create).
func (s *ActionPlanService) Suggestions(periodID string) ([]entity.AchievementRecord, error) {
	var achs []entity.AchievementRecord
	err := s.db.Preload("Indicator").
		Where("period_id = ? AND threshold_color IN ('red','yellow') AND status = 'published' AND id NOT IN (SELECT achievement_id FROM action_plans WHERE status <> 'done')",
			periodID).
		Find(&achs).Error
	return achs, err
}
