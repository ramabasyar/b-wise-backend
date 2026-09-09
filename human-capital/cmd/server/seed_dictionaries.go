package main

import (
	"github.com/rama/b-wise/human-capital/internal/adapter/logger"
	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// seedDictionaries — seed awal kamus kepegawaian & struktur org (Fase B).
// IDEMPOTEN: hanya insert kalau kosong; setelah itu milik admin HC (edit/tambah via UI).
func seedDictionaries(db *gorm.DB, lg *logger.Logger) {
	// 1. employee_types
	if cnt(&entity.EmployeeType{}, db) == 0 {
		rows := []entity.EmployeeType{
			{Code: "staff", Label: "Staff", RealmRoles: "staff", HasLevel: true, IsSystem: true, Sort: 1},
			{Code: "lecturer", Label: "Dosen", RealmRoles: "lecture", HasRank: true, IsSystem: true, Sort: 2},
			{Code: "lecturer_staff", Label: "Dosen + Staff", RealmRoles: "lecture,staff", HasLevel: true, HasRank: true, DualUnitAllowed: true, IsSystem: true, Sort: 3},
		}
		db.Create(&rows)
		lg.Info("[seed] employee_types: 3 baris")
	}

	// 2. org_unit_types
	if cnt(&entity.OrgUnitType{}, db) == 0 {
		rows := []entity.OrgUnitType{
			{Code: "university", Label: "Universitas", AllowedParents: "", IsSystem: true, Sort: 1},
			{Code: "faculty", Label: "Fakultas", AllowedParents: "university", IsSystem: true, Sort: 2},
			{Code: "study_program", Label: "Program Studi", AllowedParents: "faculty", IsSystem: true, Sort: 3},
			{Code: "functional_unit", Label: "Unit Fungsional", AllowedParents: "university", IsSystem: true, Sort: 4},
		}
		db.Create(&rows)
		lg.Info("[seed] org_unit_types: 4 baris")
	}

	// 5. structural_positions
	if cnt(&entity.StructuralPosition{}, db) == 0 {
		rows := []entity.StructuralPosition{
			{Code: "rektor", Label: "Rektor", ScopeUnitTypes: "university", IsApprovalHead: true, Sort: 1},
			{Code: "wakil_rektor", Label: "Wakil Rektor", ScopeUnitTypes: "university", IsApprovalHead: false, Sort: 2},
			{Code: "dekan", Label: "Dekan", ScopeUnitTypes: "faculty", IsApprovalHead: true, Sort: 3},
			{Code: "wakil_dekan", Label: "Wakil Dekan", ScopeUnitTypes: "faculty", IsApprovalHead: false, Sort: 4},
			{Code: "kaprodi", Label: "Ketua Program Studi", ScopeUnitTypes: "study_program", IsApprovalHead: true, Sort: 5},
			{Code: "kepala_unit", Label: "Kepala Unit", ScopeUnitTypes: "functional_unit", IsApprovalHead: true, Sort: 6},
		}
		db.Create(&rows)
		lg.Info("[seed] structural_positions: 6 baris")
	}

	// 6. Struktur org awal — HANYA kalau tabel departments kosong dari node faculty/university.
	seedOrgTree(db, lg)
}

func cnt(t interface{}, db *gorm.DB) int64 {
	var c int64
	db.Model(t).Count(&c)
	return c
}

// seedOrgTree — bangun tree awal: Universitas → {3 Fakultas (dari branches), Rektorat → unit fungsional},
// prodi di-bawah-kan ke fakultasnya. Idempoten via code.
func seedOrgTree(db *gorm.DB, lg *logger.Logger) {
	type br struct{ Code, Name string }
	type depRow struct{ ID, Code string }

	// root
	var root entity.Department
	if err := db.Where("code = ?", "UNIV").First(&root).Error; err != nil {
		root = entity.Department{Code: "UNIV", Name: "Universitas Binawan", OrgType: "university", IsGroup: true, IsActive: true}
		if err := db.Create(&root).Error; err != nil {
			lg.Error("seed org root gagal", zap.Error(err))
			return
		}
	}

	// fakultas dari DATA FILE Binawan (disetujui Mas Rama — staging & prod langkah sama;
	// admin bisa ubah/tambah via UI Pengaturan Organisasi setelahnya)
	faculties := []br{
		{Code: "FIKK", Name: "Fakultas Keperawatan dan Kebidanan"},
		{Code: "FIKST", Name: "Fakultas Ilmu Kesehatan dan Teknologi"},
		{Code: "FBIS", Name: "Fakultas Bisnis dan Ilmu Sosial"},
	}
	facByCode := map[string]string{} // code → id
	for _, b := range faculties {
		var f entity.Department
		if err := db.Where("code = ?", b.Code).First(&f).Error; err != nil {
			f = entity.Department{Code: b.Code, Name: b.Name, OrgType: "faculty", IsGroup: true, IsActive: true, ParentID: &root.ID}
			if e := db.Create(&f).Error; e != nil {
				continue
			}
		} else if f.OrgType == "" {
			f.OrgType = "faculty"
			f.ParentID = &root.ID
			db.Save(&f)
		}
		facByCode[b.Code] = f.ID
	}

	// prodi Binawan (data file — idempoten by code)
	prodi := []entity.Department{
		{Code: "KEP", Name: "S1 Keperawatan", OrgType: "study_program"},
		{Code: "KBD", Name: "D4 Kebidanan", OrgType: "study_program"},
		{Code: "FRM", Name: "D3 Farmasi", OrgType: "study_program"},
		{Code: "TLM", Name: "D4 Teknologi Laboratorium Medis", OrgType: "study_program"},
		{Code: "GZI", Name: "S1 Gizi", OrgType: "study_program"},
		{Code: "AKT", Name: "S1 Akuntansi", OrgType: "study_program"},
		{Code: "MNJ", Name: "S1 Manajemen", OrgType: "study_program"},
		{Code: "KOM", Name: "S1 Komunikasi", OrgType: "study_program"},
	}
	prodiCount := 0
	for _, pr := range prodi {
		var ex entity.Department
		if err := db.Where("code = ?", pr.Code).First(&ex).Error; err != nil {
			if facID, ok := facByCode[prodiFacCode(pr.Code)]; ok {
				pr.ParentID = &facID
			}
			if e := db.Create(&pr).Error; e == nil {
				prodiCount++
			}
		} else if ex.ParentID == nil {
			if facID, ok := facByCode[prodiFacCode(ex.Code)]; ok {
				db.Model(&entity.Department{}).Where("id = ?", ex.ID).
					Updates(map[string]interface{}{"org_type": "study_program", "parent_id": facID})
				prodiCount++
			}
		}
	}

	// unit fungsional: REK/IT/KEU (sudah ada dari Fase A) → parent rektorat? Rektorat = functional_unit langsung di bawah root.
	// Struktur: UNIV → REK (functional_unit, container) → IT, KEU, HRD, BAA, PERPUS
	var rekt entity.Department
	if err := db.Where("code = ?", "REK").First(&rekt).Error; err != nil {
		rekt = entity.Department{Code: "REK", Name: "Rektorat", OrgType: "functional_unit", IsGroup: true, IsActive: true, ParentID: &root.ID}
		db.Create(&rekt)
	} else if rekt.ParentID == nil || *rekt.ParentID != root.ID {
		rekt.OrgType = "functional_unit"
		rekt.ParentID = &root.ID
		rekt.IsGroup = true
		db.Save(&rekt)
	}

	unitBaru := []entity.Department{
		{Code: "HRD", Name: "Unit Kepegawaian (HRD)", OrgType: "functional_unit", IsActive: true, ParentID: &rekt.ID},
		{Code: "BAA", Name: "Unit Akademik & BAA", OrgType: "functional_unit", IsActive: true, ParentID: &rekt.ID},
		{Code: "PERPUS", Name: "Unit Perpustakaan", OrgType: "functional_unit", IsActive: true, ParentID: &rekt.ID},
	}
	nUnit := 0
	for _, u := range unitBaru {
		var ex entity.Department
		if err := db.Where("code = ?", u.Code).First(&ex).Error; err != nil {
			if db.Create(&u).Error == nil {
				nUnit++
			}
		}
	}

	// IT & KEU → anak Rektorat
	db.Model(&entity.Department{}).Where("code IN ?", []string{"IT", "KEU"}).
		Updates(map[string]interface{}{"org_type": "functional_unit", "parent_id": rekt.ID})

	lg.Info("[seed] org tree: root+fakultas+prodi(" + itoa(prodiCount) + ")+unit(" + itoa(nUnit) + ") baru")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := [12]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// prodiFacCode — mapping prodi→fakultas (data file, keputusan Mas Rama 22:12)
func prodiFacCode(code string) string {
	m := map[string]string{
		"KEP": "FIKK", "KBD": "FIKK",
		"FRM": "FIKST", "TLM": "FIKST", "GZI": "FIKST",
		"AKT": "FBIS", "MNJ": "FBIS", "KOM": "FBIS",
	}
	return m[code]
}
