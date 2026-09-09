package service

import (
	"errors"
	"fmt"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== TARGET ENGINE (F1) ====================

type TargetService struct {
	db *gorm.DB
}

func NewTargetService(db *gorm.DB) *TargetService { return &TargetService{db: db} }

type TargetFilter struct {
	IndicatorID string
	UnitID      string
	PeriodID    string
	Status      string
}

type UpsertTargetInput struct {
	IndicatorID     string
	UnitID          string // "institution" = level institusi
	UnitType        string // institution|branch|department
	PeriodID        string
	TargetValue     float64
	AggregationRule string
	PKRefID         *string
	Notes           string
}

func (s *TargetService) List(f TargetFilter) ([]entity.PerformanceTarget, error) {
	q := s.db.Model(&entity.PerformanceTarget{})
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
	var list []entity.PerformanceTarget
	return list, q.Preload("Indicator").Preload("Period").
		Order("created_at DESC").Find(&list).Error
}

// Upsert — target unik per (indicator, unit, period); update bila sudah ada.
func (s *TargetService) Upsert(in UpsertTargetInput, actor string) (*entity.PerformanceTarget, error) {
	if in.IndicatorID == "" || in.PeriodID == "" || in.UnitID == "" {
		return nil, fmt.Errorf("%w: indicator, unit, periode wajib", ErrValidation)
	}
	// validasi referensi
	var ind entity.IndicatorDefinition
	if err := s.db.Where("id = ?", in.IndicatorID).First(&ind).Error; err != nil {
		return nil, fmt.Errorf("%w: indikator tidak dikenal", ErrValidation)
	}
	var per entity.Period
	if err := s.db.Where("id = ?", in.PeriodID).First(&per).Error; err != nil {
		return nil, fmt.Errorf("%w: periode tidak dikenal", ErrValidation)
	}
	// F9: target boleh disiapkan sejak provisioned (perencanaan), tapi TERKUNCI saat periode closed
	if per.Status == "closed" {
		return nil, fmt.Errorf("%w: periode %s sudah ditutup - target terkunci (view-only)", ErrValidation, per.Label)
	}
	if in.UnitType == "" {
		if in.UnitID == "institution" {
			in.UnitType = "institution"
		} else {
			in.UnitType = "branch"
		}
	}

	var t entity.PerformanceTarget
	err := s.db.Where("indicator_id = ? AND unit_id = ? AND period_id = ?",
		in.IndicatorID, in.UnitID, in.PeriodID).First(&t).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		t = entity.PerformanceTarget{
			IndicatorID: in.IndicatorID, UnitID: in.UnitID, UnitType: in.UnitType,
			PeriodID: in.PeriodID, TargetValue: in.TargetValue,
			AggregationRule: in.AggregationRule, PKRefID: in.PKRefID,
			Notes: in.Notes, SetBy: actor, Status: "draft",
		}
		if err := s.db.Create(&t).Error; err != nil {
			return nil, err
		}
		return &t, nil
	}
	if err != nil {
		return nil, err
	}
	// update existing
	t.TargetValue = in.TargetValue
	t.AggregationRule = in.AggregationRule
	t.PKRefID = in.PKRefID
	t.Notes = in.Notes
	t.SetBy = actor
	if err := s.db.Save(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// Approve — pimpinan menyetujui target (status approved).
func (s *TargetService) Approve(id, approver string) error {
	var t entity.PerformanceTarget
	if err := s.db.Where("id = ?", id).First(&t).Error; err != nil {
		return ErrNotFound
	}
	// F9: approval target terkunci saat periode closed
	var per entity.Period
	if err := s.db.Select("status", "label").Where("id = ?", t.PeriodID).First(&per).Error; err == nil && per.Status == "closed" {
		return fmt.Errorf("%w: periode %s sudah ditutup - target terkunci (view-only)", ErrValidation, per.Label)
	}
	res := s.db.Model(&entity.PerformanceTarget{}).
		Where("id = ?", id).Updates(map[string]interface{}{
		"status": "approved", "approved_by": approver,
	})
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- PK-lite ----------

func (s *TargetService) ListPKDocuments(unitID string, year int) ([]entity.PKDocument, error) {
	q := s.db.Model(&entity.PKDocument{})
	if unitID != "" {
		q = q.Where("unit_id = ?", unitID)
	}
	if year > 0 {
		q = q.Where("year = ?", year)
	}
	var list []entity.PKDocument
	return list, q.Order("year DESC").Find(&list).Error
}

func (s *TargetService) CreatePKDocument(pk *entity.PKDocument, actor string) error {
	if pk.UnitID == "" || pk.Year == 0 || pk.Title == "" {
		return fmt.Errorf("%w: unit, tahun, judul PK wajib", ErrValidation)
	}
	pk.CreatedBy = actor
	return s.db.Create(pk).Error
}

// ---------- Threshold ----------

func (s *TargetService) UpsertThreshold(indicatorID string, red, yellow, green float64) error {
	var t entity.IndicatorThreshold
	err := s.db.Where("indicator_id = ?", indicatorID).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		t = entity.IndicatorThreshold{IndicatorID: indicatorID, RedBelow: red, YellowBelow: yellow, GreenMin: green}
		return s.db.Create(&t).Error
	}
	if err != nil {
		return err
	}
	t.RedBelow, t.YellowBelow, t.GreenMin = red, yellow, green
	return s.db.Save(&t).Error
}
