package service

import (
	"errors"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== DASHBOARD AGGREGATION (F3) ====================
// Role-aware dilakukan di handler (unit scope dari permission), service
// menyediakan agregasi per unit/periode + tren + export shape.

type DashboardService struct {
	db *gorm.DB
}

func NewDashboardService(db *gorm.DB) *DashboardService { return &DashboardService{db: db} }

type DashIndicator struct {
	IndicatorID string   `json:"indicator_id"`
	IkuCode     string   `json:"iku_code"`
	Name        string   `json:"name"`
	Nature      string   `json:"nature"`
	UnitID      string   `json:"unit_id"`
	Status      string   `json:"status"` // capaian status terakhir
	Value       *float64 `json:"value"`
	Target      *float64 `json:"target"`
	Pct         *float64 `json:"pct"`
	Color       string   `json:"color"`
	Evidences   int      `json:"evidences"`
	PeriodLabel string   `json:"period_label"`
	// F10: perbandingan YoY (tahun sebelumnya, annual, read-only)
	PrevValue      *float64 `json:"prev_value,omitempty"`
	Delta          *float64 `json:"delta,omitempty"`
	DeltaPct       *float64 `json:"delta_pct,omitempty"`
	Verdict        string   `json:"verdict,omitempty"` // better|worse|flat|new
	Polarity       string   `json:"polarity,omitempty"`
	FormulaChanged bool     `json:"formula_changed,omitempty"`
}

type DashSummary struct {
	UnitID          string          `json:"unit_id"`
	PeriodID        string          `json:"period_id"`
	PeriodLabel     string          `json:"period_label"`
	TotalIndicators int             `json:"total_indicators"`
	Published       int             `json:"published"`
	InProgress      int             `json:"in_progress"` // draft s.d. approved
	Missing         int             `json:"missing"`     // tanpa capaian
	Aggregated      int             `json:"aggregated"`  // dihitung dari unit anak
	AvgPct          *float64        `json:"avg_pct"`
	Red             int             `json:"red"`
	Yellow          int             `json:"yellow"`
	Green           int             `json:"green"`
	Indicators      []DashIndicator `json:"indicators"`
}

// LatestPeriod — periode dengan data capaian TERBARU berdasar status paling lanjut
// (published > approved > reviewed > submitted > draft), fallback start_date terbesar.
func (s *DashboardService) LatestPeriod(out *entity.Period) error {
	var withData string
	// F10: prioritaskan periode AKTIF (open/grace) — periode historis closed hanya via selector eksplisit.
	row := s.db.Raw(`SELECT ar.period_id FROM achievement_records ar
		JOIN periods p ON p.id = ar.period_id
		ORDER BY CASE WHEN p.status IN ('open','grace') THEN 1 ELSE 0 END DESC,
			CASE WHEN p.type = 'annual' THEN 1 ELSE 0 END DESC,
			CASE ar.status
			WHEN 'published' THEN 5 WHEN 'approved' THEN 4 WHEN 'reviewed' THEN 3
			WHEN 'submitted' THEN 2 ELSE 1 END DESC, p.start_date DESC, ar.updated_at DESC
		LIMIT 1`).Row()
	if err := row.Scan(&withData); err == nil && withData != "" {
		if err := s.db.Where("id = ?", withData).First(out).Error; err == nil {
			return nil
		}
	}
	if err := s.db.Order("start_date DESC").First(out).Error; err != nil {
		return ErrNotFound
	}
	return nil
}

var statusRank = map[string]int{"draft": 1, "submitted": 2, "reviewed": 3, "approved": 4, "published": 5, "rejected": 0}

// Build — ringkasan capaian 12 IKU utk satu unit pada satu periode.
// childUnitIDs: utk unit induk (institution) — indikator tanpa capaian level induk
// dihitung ROLLUP dari capaian ter-lanjut per unit anak (submitted ke atas).
func (s *DashboardService) Build(unitID, periodID string, childUnitIDs []string) (*DashSummary, error) {
	var period entity.Period
	if err := s.db.Where("id = ?", periodID).First(&period).Error; err != nil {
		return nil, ErrNotFound
	}

	var inds []entity.IndicatorDefinition
	s.db.Where("is_active = ?", true).Order("sort_order ASC").Find(&inds)

	// F10: siapkan data YoY — capaian unit pada periode TAHUNAN tahun sebelumnya
	prevAch := map[string]entity.AchievementRecord{}
	var prevPeriodID string
	{
		var currPer entity.Period
		if err := s.db.Where("id = ?", periodID).First(&currPer).Error; err == nil && currPer.Type == "annual" {
			var prevPer entity.Period
			if err := s.db.Where("year = ? AND type = 'annual'", currPer.Year-1).First(&prevPer).Error; err == nil {
				prevPeriodID = prevPer.ID
				var prevs []entity.AchievementRecord
				s.db.Where("unit_id = ? AND period_id = ? AND status <> 'rejected' AND status <> 'missed' AND deleted_at IS NULL",
					unitID, prevPer.ID).Find(&prevs)
				for _, a := range prevs {
					k := a.IndicatorID
					if b, ok := prevAch[k]; !ok || statusRank[a.Status] > statusRank[b.Status] {
						prevAch[k] = a
					}
				}
			}
		}
	}

	// capaian unit+periode
	var achs []entity.AchievementRecord
	s.db.Preload("Evidences").
		Where("unit_id = ? AND period_id = ?", unitID, periodID).
		Find(&achs)
	achByKey := map[string]entity.AchievementRecord{}
	for _, a := range achs {
		achByKey[a.IndicatorID] = a
	}

	sum := &DashSummary{UnitID: unitID, PeriodID: periodID, PeriodLabel: period.Label, Indicators: []DashIndicator{}}
	var pctSum float64
	var pctCount int
	for _, ind := range inds {
		di := DashIndicator{
			IndicatorID: ind.ID, IkuCode: ind.IkuCode, Name: ind.Name,
			Nature: ind.Nature, UnitID: unitID, PeriodLabel: period.Label,
		}
		if a, ok := achByKey[ind.ID]; ok {
			di.Status = a.Status
			di.Value = &a.CalculatedValue
			di.Target = a.TargetValue
			di.Pct = a.AchievementPct
			di.Color = a.ThresholdColor
			di.Evidences = len(a.Evidences)
			if a.Status == "published" {
				sum.Published++
				sum.InProgress++
			} else {
				sum.InProgress++
			}
			if a.AchievementPct != nil {
				pctSum += *a.AchievementPct
				pctCount++
			}
			switch a.ThresholdColor {
			case "red":
				sum.Red++
			case "yellow":
				sum.Yellow++
			case "green":
				sum.Green++
			}
			if pv, exists := prevAch[ind.ID]; exists {
				s.fillYoY(&di, ind, a, pv, true)
			} else {
				s.fillYoY(&di, ind, a, entity.AchievementRecord{}, false)
			}
		} else if len(childUnitIDs) > 0 {
			// ROLLUP: agregasi dari unit anak (blueprint: kaskade institusi ← fakultas)
			if agg, n := s.rollup(ind.ID, periodID, childUnitIDs); n > 0 {
				di.Status = "aggregated"
				di.Value = &agg
				// target level induk utk pct
				var tgt entity.PerformanceTarget
				if err := s.db.Where("indicator_id = ? AND unit_id = ? AND period_id = ?", ind.ID, unitID, periodID).First(&tgt).Error; err == nil && tgt.TargetValue != 0 {
					pct := agg / tgt.TargetValue * 100
					di.Target = &tgt.TargetValue
					di.Pct = &pct
					di.Color = thresholdColor(s.db, ind.ID, pct)
				}
				// F10: YoY utk baris agregat — prev dari rollup anak periode tahun lalu
				if prevPeriodID != "" {
					if pv, pn := s.rollup(ind.ID, prevPeriodID, childUnitIDs); pn > 0 {
						s.fillYoY(&di, ind, entity.AchievementRecord{}, entity.AchievementRecord{CalculatedValue: pv}, true)
					}
				}
				sum.Aggregated++
				if di.Pct != nil {
					pctSum += *di.Pct
					pctCount++
				}
				switch di.Color {
				case "red":
					sum.Red++
				case "yellow":
					sum.Yellow++
				case "green":
					sum.Green++
				}
			} else {
				sum.Missing++
			}
		} else {
			sum.Missing++
		}
		sum.TotalIndicators++
		sum.Indicators = append(sum.Indicators, di)
	}
	if pctCount > 0 {
		avg := pctSum / float64(pctCount)
		sum.AvgPct = &avg
	}
	return sum, nil
}

// Trend — pct rata-rata per unit antar periode (urut waktu).
func (s *DashboardService) Trend(unitID string, year int) ([]map[string]interface{}, error) {
	var periods []entity.Period
	q := s.db.Where("year = ?", year).Order("start_date ASC")
	if err := q.Find(&periods).Error; err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	for _, p := range periods {
		var achs []entity.AchievementRecord
		s.db.Where("unit_id = ? AND period_id = ? AND status = 'published'", unitID, p.ID).Find(&achs)
		var sum float64
		var n int
		for _, a := range achs {
			if a.AchievementPct != nil {
				sum += *a.AchievementPct
				n++
			}
		}
		row := map[string]interface{}{
			"period": p.Label, "period_id": p.ID, "published": n,
		}
		if n > 0 {
			row["avg_pct"] = sum / float64(n)
		} else {
			row["avg_pct"] = nil
		}
		out = append(out, row)
	}
	return out, nil
}

// UnitComparison — avg pct antar unit untuk satu periode (drill-down rektor→unit).
func (s *DashboardService) UnitComparison(periodID string, unitIDs []string) ([]map[string]interface{}, error) {
	out := []map[string]interface{}{}
	for _, uid := range unitIDs {
		var achs []entity.AchievementRecord
		s.db.Where("unit_id = ? AND period_id = ? AND status = 'published'", uid, periodID).Find(&achs)
		var sum float64
		var n int
		green, yellow, red := 0, 0, 0
		for _, a := range achs {
			if a.AchievementPct != nil {
				sum += *a.AchievementPct
				n++
			}
			switch a.ThresholdColor {
			case "green":
				green++
			case "yellow":
				yellow++
			case "red":
				red++
			}
		}
		row := map[string]interface{}{
			"unit_id": uid, "published": n, "green": green, "yellow": yellow, "red": red,
		}
		if n > 0 {
			row["avg_pct"] = sum / float64(n)
		} else {
			row["avg_pct"] = nil
		}
		out = append(out, row)
	}
	return out, nil
}

var _ = errors.New

// rollup — nilai agregat indikator dari unit anak: ambil record ter-lanjut
// per unit (submitted ke atas, rank status), lalu rata-rata.
func (s *DashboardService) rollup(indicatorID, periodID string, childUnitIDs []string) (float64, int) {
	var achs []entity.AchievementRecord
	s.db.Where("indicator_id = ? AND period_id = ? AND unit_id IN ? AND status IN ('submitted','reviewed','approved','published')",
		indicatorID, periodID, childUnitIDs).Find(&achs)

	best := map[string]entity.AchievementRecord{}
	for _, a := range achs {
		if b, ok := best[a.UnitID]; !ok || statusRank[a.Status] > statusRank[b.Status] {
			best[a.UnitID] = a
		}
	}
	if len(best) == 0 {
		return 0, 0
	}
	var sum float64
	for _, a := range best {
		sum += a.CalculatedValue
	}
	return sum / float64(len(best)), len(best)
}

// thresholdColor — warna berdasar pct (pakai config indikator, default 50/75).
func thresholdColor(db *gorm.DB, indicatorID string, pct float64) string {
	var th entity.IndicatorThreshold
	if db.Where("indicator_id = ?", indicatorID).First(&th).Error == nil {
		switch {
		case pct < th.RedBelow:
			return "red"
		case pct < th.YellowBelow:
			return "yellow"
		}
		return "green"
	}
	switch {
	case pct < 50:
		return "red"
	case pct < 75:
		return "yellow"
	}
	return "green"
}

// fillYoY — isi perbandingan tahun lalu (F10) pada satu baris dashboard.
// curr: capaian tahun ini (bisa nil utk agregat); prev: capaian annual tahun lalu (bisa zero-value).
func (s *DashboardService) fillYoY(di *DashIndicator, ind entity.IndicatorDefinition, curr entity.AchievementRecord, prev entity.AchievementRecord, prevExists bool) {
	di.Polarity = ind.Polarity
	if di.Polarity != "lower_is_better" {
		di.Polarity = "higher_is_better"
	}
	if di.Value == nil {
		return
	}
	if !prevExists {
		di.Verdict = "new"
		return
	}
	pv := prev.CalculatedValue
	di.PrevValue = &pv
	d := *di.Value - pv
	di.Delta = &d
	if pv != 0 {
		pct := d / pv * 100
		di.DeltaPct = &pct
	}
	if curr.FormulaID != nil && prev.FormulaID != nil && *curr.FormulaID != *prev.FormulaID {
		di.FormulaChanged = true
	}
	betterIfUp := di.Polarity != "lower_is_better"
	if di.DeltaPct != nil && *di.DeltaPct > -1 && *di.DeltaPct < 1 {
		di.Verdict = "flat"
	} else if d == 0 {
		di.Verdict = "flat"
	} else if (d > 0) == betterIfUp {
		di.Verdict = "better"
	} else {
		di.Verdict = "worse"
	}
}
