package service

import (
	"errors"
	"fmt"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

var (
	ErrNotFound   = errors.New("data tidak ditemukan")
	ErrValidation = errors.New("validasi gagal")
)

// IkuService — business logic F0: regulatory registry + indicator definitions + periods.
type IkuService struct {
	db *gorm.DB
}

func NewIkuService(db *gorm.DB) *IkuService { return &IkuService{db: db} }

// ==================== REGULATORY ====================

func (s *IkuService) ListRegulations(includeInactive bool) ([]entity.RegulatoryVersion, error) {
	q := s.db
	if !includeInactive {
		q = q.Where("status = ?", "active")
	}
	var list []entity.RegulatoryVersion
	return list, q.Order("effective_date DESC").Find(&list).Error
}

func (s *IkuService) CreateRegulation(r *entity.RegulatoryVersion) error {
	if r.RegulationCode == "" || r.Title == "" || r.EffectiveDate.IsZero() {
		return fmt.Errorf("%w: regulation_code, title, effective_date wajib", ErrValidation)
	}
	if err := s.db.Create(r).Error; err != nil {
		return err
	}
	return nil
}

// ==================== INDICATORS ====================

type IndicatorFilter struct {
	RegVersionID string
	PeriodType   string
	Nature       string
	ActiveOnly   bool
}

func (s *IkuService) ListIndicators(f IndicatorFilter) ([]entity.IndicatorDefinition, error) {
	q := s.db.Model(&entity.IndicatorDefinition{})
	if f.RegVersionID != "" {
		q = q.Where("reg_version_id = ?", f.RegVersionID)
	}
	if f.PeriodType != "" {
		q = q.Where("period_type = ?", f.PeriodType)
	}
	if f.Nature != "" {
		q = q.Where("nature = ?", f.Nature)
	}
	if f.ActiveOnly {
		q = q.Where("is_active = ?", true)
	}
	var list []entity.IndicatorDefinition
	return list, q.Preload("RegVersion").Order("sort_order ASC, iku_code ASC").Find(&list).Error
}

func (s *IkuService) GetIndicator(id string) (*entity.IndicatorDefinition, error) {
	var d entity.IndicatorDefinition
	if err := s.db.Preload("RegVersion").Where("id = ?", id).First(&d).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &d, nil
}

// ==================== PERIODS ====================

func (s *IkuService) ListPeriods(year int, ptype string) ([]entity.Period, error) {
	q := s.db.Model(&entity.Period{})
	if year > 0 {
		q = q.Where("year = ?", year)
	}
	if ptype != "" {
		q = q.Where("type = ?", ptype)
	}
	var list []entity.Period
	return list, q.Order("year DESC, sequence ASC").Find(&list).Error
}

func (s *IkuService) CreatePeriod(p *entity.Period) error {
	if p.Label == "" || p.Type == "" || p.Year == 0 {
		return fmt.Errorf("%w: label, type, year wajib", ErrValidation)
	}
	return s.db.Create(p).Error
}

// ==================== SEED (idempoten) ====================
// 12 IKU Kepmendiktisaintek 358/M/KEP/2025 — DATA, bukan kode.
// Sumber: Blueprint v2 Mas Rama Bab 1.2 (disusun dari regulasi resmi).

var seedIKU2025 = []entity.IndicatorDefinition{
	{IkuCode: "IKU-1", Name: "Angka Efisiensi Edukasi PT (AEE PT)", Nature: "wajib", PeriodType: "annual", SortOrder: 1, Description: "Efisiensi edukasi perguruan tinggi."},
	{IkuCode: "IKU-2", Name: "Lulusan Langsung Bekerja/Wirausaha/Lanjut Studi", Nature: "wajib", PeriodType: "annual", SortOrder: 2, Description: "Persentase lulusan yang langsung bekerja, berwirausaha, atau lanjut studi."},
	{IkuCode: "IKU-3", Name: "Mahasiswa Berkegiatan/Berprestasi di Luar Prodi", Nature: "wajib", PeriodType: "annual", SortOrder: 3},
	{IkuCode: "IKU-4", Name: "Dosen dengan Rekognisi Internasional", Nature: "pilihan", PeriodType: "annual", SortOrder: 4},
	{IkuCode: "IKU-5", Name: "Rasio Luaran Kerjasama dan Startup/Industri", Nature: "wajib", PeriodType: "annual", SortOrder: 5},
	{IkuCode: "IKU-6", Name: "Publikasi Bereputasi Internasional (Scopus/WoS)", Nature: "wajib", PeriodType: "annual", SortOrder: 6, ApplicablePT: "ptn-bh"},
	{IkuCode: "IKU-7", Name: "Keterlibatan dalam SDGs (SDG 1, 4, 17 + 2 pilihan)", Nature: "wajib", PeriodType: "annual", SortOrder: 7},
	{IkuCode: "IKU-8", Name: "SDM PT Terlibat Penyusunan Kebijakan", Nature: "pilihan", PeriodType: "annual", SortOrder: 8},
	{IkuCode: "IKU-9", Name: "Persentase Pendapatan Non-Pendidikan (Non-UKT)", Nature: "wajib", PeriodType: "annual", SortOrder: 9},
	{IkuCode: "IKU-10", Name: "Usulan Zona Integritas WBK/WBBM", Nature: "pilihan", PeriodType: "annual", SortOrder: 10, ApplicablePT: "ptn"},
	{IkuCode: "IKU-11", Name: "Opini WTP & Akuntabilitas Kinerja", Nature: "pilihan", PeriodType: "annual", SortOrder: 11},
	{IkuCode: "IKU-12", Name: "Perencanaan Strategis Kesejahteraan Dosen", Nature: "wajib", PeriodType: "annual", SortOrder: 12},
}

// DB — akses db (dipakai handler utk operasi kecil yang belum layak jadi method service).
func (s *IkuService) DB() *gorm.DB { return s.db }

func (s *IkuService) SeedReference() error {
	// 1) Regulation 358/2025
	var reg entity.RegulatoryVersion
	err := s.db.Where("regulation_code = ?", "Kepmendiktisaintek 358/M/KEP/2025").First(&reg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		reg = entity.RegulatoryVersion{
			RegulationCode: "Kepmendiktisaintek 358/M/KEP/2025",
			Title:          "IKU Perguruan Tinggi dan LLDIKTI (12 IKU)",
			EffectiveDate:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:         "active",
			Notes:          "Seed awal dari Blueprint B-Wise Performance Center v2. Verifikasi dokumen resmi via JDIH sebelum produksi.",
		}
		if err := s.db.Create(&reg).Error; err != nil {
			return err
		}
	}

	// 2) 12 IKU
	for i := range seedIKU2025 {
		d := seedIKU2025[i]
		d.Level = 0 // L0 nasional
		d.RegVersionID = reg.ID
		d.IsActive = true
		var count int64
		s.db.Model(&entity.IndicatorDefinition{}).
			// idempoten by iku_code GLOBAL — indikator yang pindah regulasi (compliance F6)
			// tidak boleh ter-duplikasi saat seed boot berikutnya
			Where("iku_code = ?", d.IkuCode).Count(&count)
		if count == 0 {
			s.db.Create(&d)
		}
	}

	// 3) Periode — auto-provision tahun berjalan + tahun berikutnya (F9 lifecycle).
	// Record BARU berstatus "provisioned" (menunggu admin buka); idempoten by label,
	// periode lama yang sudah open/closed tidak pernah tersentuh.
	y := time.Now().Year()
	if err := EnsureYearPeriods(s.db, y); err != nil {
		return err
	}
	if err := EnsureYearPeriods(s.db, y+1); err != nil {
		return err
	}
	return nil
}

// VerifyRegulation — F7 JDIH: tandai regulasi terverifikasi + link dokumen resmi.
func (s *IkuService) VerifyRegulation(id, documentURL string, verified bool, by string) (*entity.RegulatoryVersion, error) {
	var reg entity.RegulatoryVersion
	if err := s.db.Where("id = ?", id).First(&reg).Error; err != nil {
		return nil, ErrNotFound
	}
	patch := map[string]interface{}{}
	if documentURL != "" {
		patch["document_url"] = documentURL
	}
	if verified {
		patch["verification_status"] = "verified"
		patch["verified_at"] = time.Now()
		patch["verified_by"] = by
	} else {
		patch["verification_status"] = "unverified"
		patch["verified_at"] = nil
		patch["verified_by"] = ""
	}
	if err := s.db.Model(&reg).Updates(patch).Error; err != nil {
		return nil, err
	}
	_ = s.db.Where("id = ?", id).First(&reg).Error
	return &reg, nil
}
