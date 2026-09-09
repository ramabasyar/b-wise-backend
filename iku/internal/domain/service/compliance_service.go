package service

import (
	"math"

	"encoding/json"
	"errors"
	"fmt"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== COMPLIANCE SERVICE (F6) ====================
// Regulation Change Workflow + Impact Analyzer + Compliance Monitor.

type ComplianceService struct {
	db       *gorm.DB
	formula  *FormulaService
	Notifier *Notifier // F7: optional
}

func NewComplianceService(db *gorm.DB, formula *FormulaService) *ComplianceService {
	return &ComplianceService{db: db, formula: formula}
}

// ---------- CHANGE WORKFLOW ----------

type CreateChangeInput struct {
	Title         string
	NewRegCode    string
	NewRegTitle   string
	EffectiveDate time.Time
	Notes         string
	Items         []RegulationChangeItemInput
}

type RegulationChangeItemInput struct {
	IndicatorID       string
	NewExpression     string
	NewInputVariables []entity.FormulaInputVar
	Notes             string
}

func (s *ComplianceService) CreateChange(in CreateChangeInput, actor string) (*entity.RegulationChange, error) {
	if in.Title == "" || in.EffectiveDate.IsZero() || len(in.Items) == 0 {
		return nil, fmt.Errorf("%w: judul, tanggal efektif & minimal 1 item perubahan wajib", ErrValidation)
	}
	rc := &entity.RegulationChange{
		Title: in.Title, NewRegCode: in.NewRegCode, NewRegTitle: in.NewRegTitle,
		EffectiveDate: in.EffectiveDate, Status: "draft", RequestedBy: actor,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rc).Error; err != nil {
			return err
		}
		for _, it := range in.Items {
			// validasi: indikator ada + ekspresi compile
			var ind entity.IndicatorDefinition
			if err := tx.Where("id = ?", it.IndicatorID).First(&ind).Error; err != nil {
				return fmt.Errorf("%w: indikator item tidak dikenal", ErrValidation)
			}
			if it.NewExpression != "" {
				if err := ValidateCompile(it.NewExpression, it.NewInputVariables); err != nil {
					return fmt.Errorf("%s: %v", ind.IkuCode, err)
				}
			}
			item := entity.RegulationChangeItem{
				ChangeID: rc.ID, IndicatorID: it.IndicatorID,
				ChangeType:    "formula_update",
				NewExpression: it.NewExpression, NewInputVariables: it.NewInputVariables,
				Notes: it.Notes,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetChange(rc.ID)
}

func (s *ComplianceService) GetChange(id string) (*entity.RegulationChange, error) {
	var rc entity.RegulationChange
	if err := s.db.Preload("Items").Where("id = ?", id).First(&rc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &rc, nil
}

func (s *ComplianceService) ListChanges(includeInactive bool) ([]entity.RegulationChange, error) {
	q := s.db.Preload("Items")
	if !includeInactive {
		q = q.Where("status IN ('draft','in_review','approved')")
	}
	var list []entity.RegulationChange
	return list, q.Order("created_at DESC").Find(&list).Error
}

// TransitionChange — draft → in_review → approved → active (atau rejected).
func (s *ComplianceService) TransitionChange(id, to, actor, notes string) (*entity.RegulationChange, error) {
	rc, err := s.GetChange(id)
	if err != nil {
		return nil, err
	}
	allowed := map[string][]string{
		"draft":     {"in_review", "rejected"},
		"in_review": {"approved", "rejected"},
		"approved":  {"active"},
		"rejected":  {"draft"},
	}
	ok := false
	for _, t := range allowed[rc.Status] {
		if t == to {
			ok = true
		}
	}
	if !ok {
		return nil, fmt.Errorf("%w: transisi %s → %s tidak diizinkan", ErrValidation, rc.Status, to)
	}
	patch := map[string]interface{}{"status": to}
	now := time.Now()
	switch to {
	case "in_review":
	case "approved":
		patch["approved_by"], patch["approved_at"] = actor, now
	case "active":
		patch["approved_by"] = rc.ApprovedBy
		patch["activated_at"] = now
	case "rejected":
		if notes == "" {
			return nil, fmt.Errorf("%w: penolakan wajib catatan", ErrValidation)
		}
	}
	if notes != "" {
		patch["review_notes"] = notes
	}

	// Patch status + efek aktivasi HARUS atomik (jangan active tanpa efek nyata)
	if to == "active" {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(rc).Updates(patch).Error; err != nil {
				return err
			}
			if err := s.applyChangeTx(tx, rc, actor); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		// F7: aktivasi regulasi → broadcast operator/pimpinan
		if s.Notifier != nil {
			go s.Notifier.NotifyAdmins("regulation.activated",
				"Regulasi baru AKTIF: "+rc.Title,
				fmt.Sprintf("Perubahan regulasi \"%s\" telah diaktivasi oleh %s.\nRegulasi lama otomatis superseded; formula baru aktif untuk periode berikutnya. Capaian historis TIDAK berubah (immutable).", rc.Title, actor),
				"regulation_change", rc.ID)
		}
		return s.GetChange(id)
	}
	if err := s.db.Model(rc).Updates(patch).Error; err != nil {
		return nil, err
	}
	return s.GetChange(id)
}

// applyChangeTx — efek aktivasi (dijalankan DALAM transaction TransitionChange):
// 1) daftarkan RegulatoryVersion baru (active) + supersede versi lama
// 2) per item: buat FormulaVersion baru (versi naik) & aktifkan (lama auto-retire)
func (s *ComplianceService) applyChangeTx(tx *gorm.DB, rc *entity.RegulationChange, actor string) error {
	regCode := rc.NewRegCode
	if regCode == "" {
		regCode = fmt.Sprintf("Regulasi-%s", rc.ID[:8])
	}
	var reg entity.RegulatoryVersion
	err := tx.Where("regulation_code = ?", regCode).First(&reg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		reg = entity.RegulatoryVersion{
			RegulationCode: regCode,
			Title:          orDefaultStr(rc.NewRegTitle, rc.Title),
			EffectiveDate:  rc.EffectiveDate,
			Status:         "active",
			Notes:          fmt.Sprintf("Diaktivasi dari change %s", rc.ID[:8]),
			CreatedBy:      actor,
		}
		if err := tx.Create(&reg).Error; err != nil {
			return err
		}
	}
	// supersede regulasi aktif lain
	if err := tx.Model(&entity.RegulatoryVersion{}).
		Where("status = 'active' AND id <> ?", reg.ID).
		Update("status", "superseded").Error; err != nil {
		return err
	}

	for _, item := range rc.Items {
		if item.NewExpression == "" {
			continue
		}
		var maxVer int64
		tx.Model(&entity.FormulaVersion{}).Where("indicator_id = ?", item.IndicatorID).
			Select("COALESCE(MAX(version_number),0)").Scan(&maxVer)
		nf := entity.FormulaVersion{
			IndicatorID:     item.IndicatorID,
			VersionNumber:   int(maxVer) + 1,
			Expression:      item.NewExpression,
			InputVariables:  item.NewInputVariables,
			RoundingRule:    "2dp",
			ValidationRules: "{}", // jsonb: string kosong invalid
			Status:          "active",
			ActiveFrom:      &rc.EffectiveDate,
			Notes:           fmt.Sprintf("Aktivasi change regulasi %s", rc.Title),
			CreatedBy:       actor,
		}
		if err := tx.Create(&nf).Error; err != nil {
			return err
		}
		// retire formula aktif lain
		if err := tx.Model(&entity.FormulaVersion{}).
			Where("indicator_id = ? AND status = 'active' AND id <> ?", item.IndicatorID, nf.ID).
			Update("status", "retired").Error; err != nil {
			return err
		}
		// definisi indikator: pindahkan ke regulasi baru (valid_from)
		if err := tx.Model(&entity.IndicatorDefinition{}).Where("id = ?", item.IndicatorID).
			Updates(map[string]interface{}{"reg_version_id": reg.ID, "valid_from": rc.EffectiveDate}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ---------- IMPACT ANALYZER ----------

type ImpactRow struct {
	IndicatorCode string   `json:"indicator_code"`
	UnitID        string   `json:"unit_id"`
	PeriodLabel   string   `json:"period_label"`
	OldValue      *float64 `json:"old_value"`
	NewValue      *float64 `json:"new_value"`
	Delta         *float64 `json:"delta"`
	OldTarget     *float64 `json:"old_target"`
	OldPct        *float64 `json:"old_pct"`
	NewPct        *float64 `json:"new_pct"`
	Status        string   `json:"status"`
	Warnings      []string `json:"warnings,omitempty"`
}

type ImpactReport struct {
	ChangeID    string      `json:"change_id"`
	Title       string      `json:"title"`
	SimulatedAt string      `json:"simulated_at"`
	Rows        []ImpactRow `json:"rows"`
	Summary     struct {
		AffectedCapaian int `json:"affected_capaian"`
		Improved        int `json:"improved"`
		Degraded        int `json:"degraded"`
		Unchanged       int `json:"unchanged"`
		Failed          int `json:"failed"` // recompute error (mis. variabel hilang)
	} `json:"summary"`
}

// AnalyzeImpact — simulasi formula DRAFT pada capaian historis (tanpa menyimpan).
func (s *ComplianceService) AnalyzeImpact(changeID string) (*ImpactReport, error) {
	rc, err := s.GetChange(changeID)
	if err != nil {
		return nil, err
	}
	report := &ImpactReport{ChangeID: rc.ID, Title: rc.Title, SimulatedAt: time.Now().Format(time.RFC3339)}

	for _, item := range rc.Items {
		if item.NewExpression == "" {
			continue
		}
		// capaian historis indikator ini (semua status >= submitted)
		var achs []entity.AchievementRecord
		s.db.Preload("Indicator").Preload("Period").
			Where("indicator_id = ? AND status IN ('submitted','reviewed','approved','published')", item.IndicatorID).
			Find(&achs)

		draftF := &entity.FormulaVersion{
			Expression:     item.NewExpression,
			InputVariables: item.NewInputVariables,
			RoundingRule:   "2dp",
		}
		for _, a := range achs {
			row := ImpactRow{
				IndicatorCode: a.Indicator.IkuCode,
				UnitID:        a.UnitID,
				PeriodLabel:   a.Period.Label,
				OldValue:      &a.CalculatedValue,
				OldTarget:     a.TargetValue,
				OldPct:        a.AchievementPct,
				Status:        a.Status,
			}
			newVal, err := Evaluate(draftF, a.RawData)
			if err != nil {
				row.Warnings = append(row.Warnings, "recompute gagal: "+err.Error())
				report.Summary.Failed++
			} else if math.IsInf(newVal, 0) || math.IsNaN(newVal) {
				// div-by-zero (mis. variabel baru belum ada di raw historis) → failed, bukan +Inf
				row.Warnings = append(row.Warnings, "hasil tidak terdefinisi (pembagian nol / variabel baru belum ada di data historis)")
				row.NewValue = nil
				report.Summary.Failed++
			} else {
				row.NewValue = &newVal
				d := newVal - a.CalculatedValue
				row.Delta = &d
				if a.TargetValue != nil && *a.TargetValue != 0 {
					np := newVal / *a.TargetValue * 100
					row.NewPct = &np
				}
				switch {
				case d > 0.009:
					report.Summary.Improved++
				case d < -0.009:
					report.Summary.Degraded++
				default:
					report.Summary.Unchanged++
				}
			}
			report.Rows = append(report.Rows, row)
			report.Summary.AffectedCapaian++
		}
	}
	return report, nil
}

// ---------- COMPLIANCE MONITOR ----------

type ComplianceRow struct {
	IndicatorID    string `json:"indicator_id"`
	IkuCode        string `json:"iku_code"`
	Name           string `json:"name"`
	Nature         string `json:"nature"`
	HasFormula     bool   `json:"has_formula"`
	HasTarget      bool   `json:"has_target"`
	HasAchievement bool   `json:"has_achievement"`
	AchieveStatus  string `json:"achieve_status"`
	Published      bool   `json:"published"`
	Level          string `json:"level"` // ok|warn|missing
	Message        string `json:"message"`
}

type ComplianceReport struct {
	PeriodID    string          `json:"period_id"`
	PeriodLabel string          `json:"period_label"`
	Rows        []ComplianceRow `json:"rows"`
	Summary     struct {
		Total   int `json:"total"`
		Ok      int `json:"ok"`
		Warn    int `json:"warn"`
		Missing int `json:"missing"`
	} `json:"summary"`
}

// Monitor — IKU WAJIB: punya formula? target? capaian? published? → peringatan dini.
func (s *ComplianceService) Monitor(periodID string) (*ComplianceReport, error) {
	var period entity.Period
	if periodID == "" {
		// default: periode dgn capaian ter-advance (konsisten dgn dashboard)
		var withData string
		row := s.db.Raw(`SELECT ar.period_id FROM achievement_records ar
			JOIN periods p ON p.id = ar.period_id
			ORDER BY CASE ar.status WHEN 'published' THEN 5 WHEN 'approved' THEN 4 WHEN 'reviewed' THEN 3
			WHEN 'submitted' THEN 2 ELSE 1 END DESC, ar.updated_at DESC LIMIT 1`).Row()
		if err := row.Scan(&withData); err == nil && withData != "" {
			periodID = withData
		}
	}
	if periodID == "" {
		if err := s.db.Order("start_date DESC").First(&period).Error; err != nil {
			return nil, ErrNotFound
		}
	} else if err := s.db.Where("id = ?", periodID).First(&period).Error; err != nil {
		return nil, ErrNotFound
	}

	var inds []entity.IndicatorDefinition
	s.db.Where("is_active = ? AND nature = 'wajib'", true).Order("sort_order").Find(&inds)

	rep := &ComplianceReport{PeriodID: period.ID, PeriodLabel: period.Label}
	for _, ind := range inds {
		row := ComplianceRow{IndicatorID: ind.ID, IkuCode: ind.IkuCode, Name: ind.Name, Nature: ind.Nature}

		if _, err := s.formula.GetActive(ind.ID); err == nil {
			row.HasFormula = true
		}
		var tgtCount int64
		s.db.Model(&entity.PerformanceTarget{}).Where("indicator_id = ? AND period_id = ?", ind.ID, period.ID).Count(&tgtCount)
		row.HasTarget = tgtCount > 0

		var ach entity.AchievementRecord
		err := s.db.Where("indicator_id = ? AND period_id = ?", ind.ID, period.ID).First(&ach).Error
		if err == nil {
			row.HasAchievement = true
			row.AchieveStatus = ach.Status
			row.Published = ach.Status == "published"
		}

		switch {
		case !row.HasFormula:
			row.Level = "missing"
			row.Message = "Belum ada formula aktif — tidak terukur"
		case !row.HasTarget:
			row.Level = "missing"
			row.Message = "Belum ada target untuk periode ini"
		case !row.HasAchievement:
			row.Level = "warn"
			row.Message = "Target ada, capaian belum diisi"
		case !row.Published:
			row.Level = "warn"
			row.Message = fmt.Sprintf("Capaian masih %s", row.AchieveStatus)
		default:
			row.Level = "ok"
			row.Message = "Lengkap"
		}
		switch row.Level {
		case "ok":
			rep.Summary.Ok++
		case "warn":
			rep.Summary.Warn++
		default:
			rep.Summary.Missing++
		}
		rep.Summary.Total++
		rep.Rows = append(rep.Rows, row)
	}
	return rep, nil
}

var _ = json.Marshal
