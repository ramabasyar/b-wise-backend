package service

import (
	"fmt"

	"gorm.io/gorm"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
)

// SetupService — agregat status setup IKU: panduan urutan konfigurasi
// (periode -> indikator+formula -> target -> capaian) untuk ditampilkan
// sebagai banner/stepper di portal. Read-only, tanpa efek samping.
type SetupService struct{ db *gorm.DB }

func NewSetupService(db *gorm.DB) *SetupService { return &SetupService{db: db} }

// SetupStep — satu langkah setup beserta statusnya.
type SetupStep struct {
	Key         string `json:"key"`
	Order       int    `json:"order"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Route       string `json:"route"`
	Permission  string `json:"permission"`
	Done        bool   `json:"done"`
	Count       int    `json:"count"`
}

// SetupStatus — ringkasan setup untuk frontend.
type SetupStatus struct {
	Steps          []SetupStep `json:"steps"`
	NextStep       *SetupStep  `json:"next_step,omitempty"`
	Complete       bool        `json:"complete"`
	Periods        int         `json:"periods"`
	Indicators     int         `json:"indicators"`
	ActiveFormulas int         `json:"active_formulas"`
	Targets        int         `json:"targets"`
	Achievements   int         `json:"achievements"`
}

// GetStatus menghitung kemajuan setup dari data existing.
func (s *SetupService) GetStatus() (*SetupStatus, error) {
	st := &SetupStatus{}
	var nPeriods, nIndicators, nFormulas, nTargets, nAchievements int64
	if err := s.db.Model(&entity.Period{}).Count(&nPeriods).Error; err != nil {
		return nil, fmt.Errorf("hitung periode: %w", err)
	}
	if err := s.db.Model(&entity.IndicatorDefinition{}).Count(&nIndicators).Error; err != nil {
		return nil, fmt.Errorf("hitung indikator: %w", err)
	}
	if err := s.db.Model(&entity.FormulaVersion{}).Where("status = ?", "active").Count(&nFormulas).Error; err != nil {
		return nil, fmt.Errorf("hitung formula aktif: %w", err)
	}
	if err := s.db.Model(&entity.PerformanceTarget{}).Count(&nTargets).Error; err != nil {
		return nil, fmt.Errorf("hitung target: %w", err)
	}
	if err := s.db.Model(&entity.AchievementRecord{}).Count(&nAchievements).Error; err != nil {
		return nil, fmt.Errorf("hitung capaian: %w", err)
	}
	st.Periods, st.Indicators, st.ActiveFormulas = int(nPeriods), int(nIndicators), int(nFormulas)
	st.Targets, st.Achievements = int(nTargets), int(nAchievements)

	st.Steps = []SetupStep{
		{
			Key:   "periods",
			Order: 1, Label: "Buat Periode Pengukuran",
			Description: "Periode (mis. TW I 2026) adalah wadah seluruh pengukuran IKU — buat dan buka periode dulu.",
			Route: "/dashboard/iku/periods", Permission: "periods.write",
			Done: st.Periods > 0, Count: st.Periods,
		},
		{
			Key:   "indicators",
			Order: 2, Label: "Definisikan Indikator & Aktifkan Formula",
			Description: "Master indikator (IKU/IKT/KPI) + minimal satu versi formula berstatus aktif.",
			Route: "/dashboard/iku/indicators", Permission: "indicators.write",
			Done: st.Indicators > 0 && st.ActiveFormulas > 0, Count: st.ActiveFormulas,
		},
		{
			Key:   "targets",
			Order: 3, Label: "Tetapkan Target (PK)",
			Description: "Target per indikator per unit untuk periode terpilih — dasar perhitungan capaian.",
			Route: "/dashboard/iku/targets", Permission: "targets.write",
			Done: st.Targets > 0, Count: st.Targets,
		},
		{
			Key:   "achievements",
			Order: 4, Label: "Input / Impor Capaian",
			Description: "Realisasi diinput manual atau lewat impor — butuh periode, indikator, dan target yang siap.",
			Route: "/dashboard/iku/achievements", Permission: "achievements.write",
			Done: st.Achievements > 0, Count: st.Achievements,
		},
	}
	for i := range st.Steps {
		if !st.Steps[i].Done {
			st.NextStep = &st.Steps[i]
			break
		}
	}
	st.Complete = st.NextStep == nil
	return st, nil
}
