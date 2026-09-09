package service

import (
	"errors"
	"fmt"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== ACHIEVEMENT SERVICE (F2 — MVP core) ====================
// Input raw (manual/import) → kalkulasi formula aktif → workflow RACI:
// draft → submitted → reviewed → approved → published (rejected → draft).

type AchievementService struct {
	db       *gorm.DB
	formula  *FormulaService
	Notifier *Notifier // F7: optional
}

func NewAchievementService(db *gorm.DB, formula *FormulaService) *AchievementService {
	return &AchievementService{db: db, formula: formula}
}

var allowedTransitions = map[string][]string{
	"draft":     {"submitted"},
	"submitted": {"reviewed", "rejected"},
	"reviewed":  {"approved", "rejected"},
	"approved":  {"published"},
	"rejected":  {"submitted"},
	"published": {},
}

func canTransition(from, to string) bool {
	for _, t := range allowedTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

type AchievementFilter struct {
	IndicatorID string
	UnitID      string
	PeriodID    string
	Status      string
}

func (s *AchievementService) List(f AchievementFilter) ([]entity.AchievementRecord, error) {
	q := s.db.Model(&entity.AchievementRecord{})
	if f.IndicatorID != "" {
		q = q.Where("indicator_id = ?", f.IndicatorID)
	}
	if f.UnitID != "" {
		q = q.Where("unit_id = ?", f.UnitID)
	}
	if f.PeriodID != "" {
		q = q.Where("period_id = ?", f.PeriodID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var list []entity.AchievementRecord
	return list, q.Preload("Indicator").Preload("Period").Preload("Evidences").
		Order("updated_at DESC").Limit(500).Find(&list).Error
}

func (s *AchievementService) Get(id string) (*entity.AchievementRecord, []entity.WorkflowLog, error) {
	var a entity.AchievementRecord
	if err := s.db.Preload("Indicator").Preload("Period").Preload("Evidences").
		Where("id = ?", id).First(&a).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	var logs []entity.WorkflowLog
	s.db.Where("achievement_id = ?", id).Order("created_at ASC").Find(&logs)
	return &a, logs, nil
}

// UpsertInput — input raw data (draft). raw_data immutable setelah status != draft.
type UpsertAchievementInput struct {
	IndicatorID string
	UnitID      string // institution | branch id
	UnitType    string
	PeriodID    string
	RawData     map[string]float64
	Source      string // manual|import|connector
}

func (s *AchievementService) recalc(a *entity.AchievementRecord) error {
	// formula aktif
	f, err := s.formula.GetActive(a.IndicatorID)
	if err != nil {
		return fmt.Errorf("indikator belum punya formula aktif: %w", err)
	}
	val, err := Evaluate(f, a.RawData)
	if err != nil {
		return fmt.Errorf("kalkulasi gagal: %w", err)
	}
	a.FormulaID = &f.ID
	a.CalculatedValue = val

	// target snapshot + pct + threshold color
	var tgt entity.PerformanceTarget
	if err := s.db.Where("indicator_id = ? AND unit_id = ? AND period_id = ?",
		a.IndicatorID, a.UnitID, a.PeriodID).First(&tgt).Error; err == nil && tgt.TargetValue != 0 {
		pct := val / tgt.TargetValue * 100
		a.AchievementPct = &pct
		a.TargetValue = &tgt.TargetValue

		var th entity.IndicatorThreshold
		color := "green"
		if s.db.Where("indicator_id = ?", a.IndicatorID).First(&th).Error == nil {
			switch {
			case pct < th.RedBelow:
				color = "red"
			case pct < th.YellowBelow:
				color = "yellow"
			}
		} else {
			switch { // default
			case pct < 50:
				color = "red"
			case pct < 75:
				color = "yellow"
			}
		}
		a.ThresholdColor = color
	} else {
		a.AchievementPct = nil
		a.TargetValue = nil
		a.ThresholdColor = ""
	}
	return nil
}

// Upsert — create/update DRAFT capaian + kalkulasi otomatis.
func (s *AchievementService) Upsert(in UpsertAchievementInput, actor string) (*entity.AchievementRecord, error) {
	if in.IndicatorID == "" || in.UnitID == "" || in.PeriodID == "" || len(in.RawData) == 0 {
		return nil, fmt.Errorf("%w: indikator, unit, periode, raw_data wajib", ErrValidation)
	}
	if in.Source == "" {
		in.Source = "manual"
	}
	// F9: periode harus open (grace = input terlambat masih diterima)
	var per entity.Period
	if err := s.db.Where("id = ?", in.PeriodID).First(&per).Error; err != nil {
		return nil, fmt.Errorf("%w: periode tidak dikenal", ErrValidation)
	}
	if per.Status == "provisioned" {
		return nil, fmt.Errorf("%w: periode %s belum dibuka oleh admin", ErrValidation, per.Label)
	}
	if per.Status != "open" && per.Status != "grace" {
		return nil, fmt.Errorf("%w: periode %s sudah ditutup", ErrValidation, per.Label)
	}

	var a entity.AchievementRecord
	err := s.db.Where("indicator_id = ? AND unit_id = ? AND period_id = ?",
		in.IndicatorID, in.UnitID, in.PeriodID).First(&a).Error

	isNew := errors.Is(err, gorm.ErrRecordNotFound)
	if isNew {
		a = entity.AchievementRecord{
			IndicatorID: in.IndicatorID, UnitID: in.UnitID, PeriodID: in.PeriodID,
			UnitType: orDefaultStr(in.UnitType, "institution"), RawData: in.RawData,
			Source: in.Source, Status: "draft", CreatedBy: actor,
		}
	} else if err != nil {
		return nil, err
	} else if a.Status != "draft" && a.Status != "rejected" {
		return nil, fmt.Errorf("%w: raw_data terkunci (status %s) — capaian sudah diproses", ErrValidation, a.Status)
	} else {
		a.RawData = in.RawData
		if in.Source != "" {
			a.Source = in.Source
		}
	}

	if err := s.recalc(&a); err != nil {
		return nil, err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&a).Error; err != nil {
			return err
		}
		return tx.Create(&entity.WorkflowLog{
			AchievementID: a.ID, Action: ternary(isNew, "created", "recalculated"),
			ActorID: actor, Notes: fmt.Sprintf("source=%s calculated=%.4f", a.Source, a.CalculatedValue),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Transition — pindah status workflow (submitted/reviewed/approved/published/rejected) + log.
func (s *AchievementService) Transition(id, to, actor, role, notes, ip string) (*entity.AchievementRecord, error) {
	var a entity.AchievementRecord
	if err := s.db.Where("id = ?", id).First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	// F9: periode closed -> workflow beku (escape hatch: reopen periode oleh admin)
	var per entity.Period
	if err := s.db.Select("id", "status", "label").Where("id = ?", a.PeriodID).First(&per).Error; err == nil && per.Status == "closed" {
		return nil, fmt.Errorf("%w: periode %s sudah ditutup - workflow dibekukan (reopen periode bila perlu)", ErrValidation, per.Label)
	}
	if !canTransition(a.Status, to) {
		return nil, fmt.Errorf("%w: transisi %s → %s tidak diizinkan", ErrValidation, a.Status, to)
	}
	now := time.Now()
	patch := map[string]interface{}{"status": to}
	switch to {
	case "submitted":
		patch["submitted_by"], patch["submitted_at"] = actor, now
	case "reviewed":
		patch["reviewed_by"], patch["reviewed_at"] = actor, now
	case "approved":
		patch["approved_by"], patch["approved_at"] = actor, now
	case "published":
		patch["published_at"] = now
	case "rejected":
		if notes == "" {
			return nil, fmt.Errorf("%w: penolakan wajib ada catatan", ErrValidation)
		}
	}
	if notes != "" {
		patch["review_notes"] = notes
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&entity.AchievementRecord{}).Where("id = ?", id).Updates(patch).Error; err != nil {
			return err
		}
		return tx.Create(&entity.WorkflowLog{
			AchievementID: id, Action: to, ActorID: actor, ActorRole: role,
			Notes: notes, IPAddress: ip,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	_ = s.db.Preload("Indicator").Preload("Period").Where("id = ?", id).First(&a)

	// F7: notifikasi eskalasi — capaian ditolak → kabari pengaju
	if to == "rejected" && s.Notifier != nil {
		uid := ""
		if a.SubmittedBy != nil {
			uid = *a.SubmittedBy
		}
		s.Notifier.Notify([]string{uid}, "achievement.rejected",
			fmt.Sprintf("Capaian %s ditolak", a.Indicator.IkuCode),
			fmt.Sprintf("Capaian %s (%s) untuk periode %s DITOLAK.\nCatatan reviewer: %s\nSilakan revisi dan ajukan ulang.",
				a.Indicator.IkuCode, a.Indicator.Name, a.Period.Label, notes),
			"achievement", a.ID)
	}
	return &a, nil
}

// AttachEvidence — catat evidence row (file sudah di storage).
func (s *AchievementService) AttachEvidence(e *entity.EvidenceDocument) error {
	var a entity.AchievementRecord
	if err := s.db.Where("id = ?", e.AchievementID).First(&a).Error; err != nil {
		return ErrNotFound
	}
	// evidence boleh ditambah selama belum published
	if a.Status == "published" {
		return fmt.Errorf("%w: capaian sudah dipublikasi", ErrValidation)
	}
	// F9: periode closed -> evidence beku (integritas tutup buku, view-only)
	var evPer entity.Period
	if err := s.db.Select("status", "label").Where("id = ?", a.PeriodID).First(&evPer).Error; err == nil && evPer.Status == "closed" {
		return fmt.Errorf("%w: periode %s sudah ditutup - evidence terkunci (view-only)", ErrValidation, evPer.Label)
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(e).Error; err != nil {
			return err
		}
		return tx.Create(&entity.WorkflowLog{
			AchievementID: e.AchievementID, Action: "evidence_added", ActorID: e.UploadedBy,
			Notes: fmt.Sprintf("file=%s (%d bytes, driver=%s)", e.FileName, e.FileSize, e.StorageDriver),
		}).Error
	})
}

func (s *AchievementService) ListEvidences(achievementID string) ([]entity.EvidenceDocument, error) {
	var list []entity.EvidenceDocument
	return list, s.db.Where("achievement_id = ?", achievementID).Order("uploaded_at DESC").Find(&list).Error
}

func (s *AchievementService) DB() *gorm.DB { return s.db }

func orDefaultStr(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
func ternary(b bool, a, c string) string {
	if b {
		return a
	}
	return c
}
