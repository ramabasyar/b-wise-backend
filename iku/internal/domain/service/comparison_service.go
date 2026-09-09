package service

import (
	"fmt"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== YOY COMPARISON SERVICE (F10) ====================
// Perbandingan capaian tahun berjalan vs tahun sebelumnya (read-only).
// Verdict "membaik/memburuk" berbasis POLARITAS indikator, bukan naik/turun mentah:
// IKU-1 (AEE) lower_is_better — nilai turun justru membaik.

var cmpStatusRank = map[string]int{"draft": 1, "submitted": 2, "reviewed": 3, "approved": 4, "published": 5}

// ComparisonRow — satu pasangan (indikator, unit) tahun N vs N-1.
type ComparisonRow struct {
	IndicatorID    string   `json:"indicator_id"`
	IkuCode        string   `json:"iku_code"`
	Name           string   `json:"name"`
	UnitID         string   `json:"unit_id"`
	UnitType       string   `json:"unit_type"`
	Polarity       string   `json:"polarity"`
	PrevValue      *float64 `json:"prev_value"` // capaian terbaik tahun N-1 (annual)
	PrevStatus     string   `json:"prev_status,omitempty"`
	CurrValue      *float64 `json:"curr_value"` // capaian terbaik tahun N (annual)
	CurrStatus     string   `json:"curr_status,omitempty"`
	Delta          *float64 `json:"delta,omitempty"`     // curr - prev
	DeltaPct       *float64 `json:"delta_pct,omitempty"` // (curr-prev)/prev*100
	Verdict        string   `json:"verdict"`             // better|worse|flat|new|no_curr
	FormulaChanged bool     `json:"formula_changed"`     // formula beda antar tahun — perbandingan indikatif
	Note           string   `json:"note,omitempty"`
}

type ComparisonService struct{ db *gorm.DB }

func NewComparisonService(db *gorm.DB) *ComparisonService { return &ComparisonService{db: db} }

// annualPeriod — periode tahunan tahun y (boleh closed/historis).
func (s *ComparisonService) annualPeriod(year int) (*entity.Period, error) {
	var p entity.Period
	if err := s.db.Where("year = ? AND type = 'annual'", year).First(&p).Error; err != nil {
		return nil, ErrNotFound
	}
	return &p, nil
}

// bestByPair — capaian status ter-advance per (indicator, unit) pada satu periode.
func (s *ComparisonService) bestByPair(periodID string) map[string]entity.AchievementRecord {
	var achs []entity.AchievementRecord
	s.db.Where("period_id = ? AND status <> 'rejected' AND status <> 'missed' AND deleted_at IS NULL", periodID).Find(&achs)
	best := map[string]entity.AchievementRecord{}
	for _, a := range achs {
		k := a.IndicatorID + "|" + a.UnitID
		if b, ok := best[k]; !ok || cmpStatusRank[a.Status] > cmpStatusRank[b.Status] {
			best[k] = a
		}
	}
	return best
}

// CompareYoY — baris perbandingan utk tahun `year` vs year-1.
// unitID kosong = semua unit (institusi + fakultas). indicatorID kosong = semua indikator.
func (s *ComparisonService) CompareYoY(year int, unitID, indicatorID string) ([]ComparisonRow, error) {
	prevPer, errPrev := s.annualPeriod(year - 1)
	currPer, errCurr := s.annualPeriod(year)
	if errCurr != nil {
		return nil, fmt.Errorf("periode tahunan %d belum ada", year)
	}

	prev := map[string]entity.AchievementRecord{}
	if errPrev == nil {
		prev = s.bestByPair(prevPer.ID)
	}
	curr := s.bestByPair(currPer.ID)

	var inds []entity.IndicatorDefinition
	s.db.Where("is_active = ?", true).Order("sort_order ASC").Find(&inds)

	// pasangan yang relevan: punya capaian tahun ini ATAU tahun lalu
	rows := []ComparisonRow{}
	for _, ind := range inds {
		if indicatorID != "" && ind.ID != indicatorID {
			continue
		}
		units := map[string]string{} // unitID -> unitType
		for _, a := range curr {
			if a.IndicatorID == ind.ID {
				units[a.UnitID] = a.UnitType
			}
		}
		for _, a := range prev {
			if a.IndicatorID == ind.ID {
				units[a.UnitID] = a.UnitType
			}
		}
		pol := ind.Polarity
		if pol != "lower_is_better" {
			pol = "higher_is_better"
		}
		for uid, utype := range units {
			if unitID != "" && unitID != uid {
				continue
			}
			r := ComparisonRow{
				IndicatorID: ind.ID, IkuCode: ind.IkuCode, Name: ind.Name,
				UnitID: uid, UnitType: utype, Polarity: pol, Verdict: "no_curr",
			}
			if p, ok := prev[ind.ID+"|"+uid]; ok {
				v := p.CalculatedValue
				r.PrevValue, r.PrevStatus = &v, p.Status
			}
			if c, ok := curr[ind.ID+"|"+uid]; ok {
				v := c.CalculatedValue
				r.CurrValue, r.CurrStatus = &v, c.Status
				if r.PrevValue != nil && c.FormulaID != nil {
					if pp, ok2 := prev[ind.ID+"|"+uid]; ok2 && pp.FormulaID != nil && *pp.FormulaID != *c.FormulaID {
						r.FormulaChanged = true
						r.Note = "formula berubah antar tahun — perbandingan indikatif"
					}
				}
			}
			if r.CurrValue != nil && r.PrevValue != nil {
				d := *r.CurrValue - *r.PrevValue
				r.Delta = &d
				if *r.PrevValue != 0 {
					pct := d / *r.PrevValue * 100
					r.DeltaPct = &pct
				}
			}
			r.Verdict = verdictOf(r)
			if r.Verdict == "flat" {
				r.Note = "perubahan < 1% — relatif stabil"
			}
			rows = append(rows, r)
		}
	}
	return rows, nil
}

// verdictOf — keputusan membaik/memburuk berbasis polaritas (Delta/DeltaPct sudah diisi).
func verdictOf(r ComparisonRow) string {
	if r.CurrValue == nil {
		return "no_curr"
	}
	if r.PrevValue == nil {
		return "new"
	}
	if r.DeltaPct != nil && *r.DeltaPct > -1 && *r.DeltaPct < 1 {
		return "flat"
	}
	if r.Delta == nil || *r.Delta == 0 {
		return "flat"
	}
	betterIfUp := r.Polarity != "lower_is_better"
	up := *r.Delta > 0
	if betterIfUp == up {
		return "better"
	}
	return "worse"
}
