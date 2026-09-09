package entity

import "time"

// ==================== KAMUS KEPEGAWAIAN (Fase B — dynamic master data) ====================
// PRINSIP: semua taksonomi organisasi = data yang dikelola admin HC via UI,
// BUKAN enum di kode. Seed hanya data awal (idempoten), admin bebas menambah/mengubah.

// EmployeeType — tipe pegawai + konfigurasi behavior wizard & realm SSO.
type EmployeeType struct {
	Code            string `json:"code" gorm:"type:varchar(30);primaryKey"` // staff, lecturer, lecturer_staff
	Label           string `json:"label" gorm:"type:varchar(100);not null"`
	RealmRoles      string `json:"realm_roles" gorm:"type:varchar(200)"` // CSV slug realm SSO, mis. "lecture,staff"
	HasLevel        bool   `json:"has_level"`                            // tampilkan pilihan employment level?
	HasRank         bool   `json:"has_rank"`                             // tampilkan pilihan jenjang akademik?
	DualUnitAllowed bool   `json:"dual_unit_allowed"`                    // boleh unit kedua (dosen+staff)?
	IsSystem        bool   `json:"is_system"`                            // tidak boleh dihapus (boleh edit label)
	Sort            int    `json:"sort"`
	IsActive        bool   `json:"is_active" gorm:"default:true"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// OrgUnitType — jenis node organisasi + aturan parent (tree rule as data).
type OrgUnitType struct {
	Code           string `json:"code" gorm:"type:varchar(30);primaryKey"` // university, faculty, study_program, functional_unit
	Label          string `json:"label" gorm:"type:varchar(100);not null"`
	AllowedParents string `json:"allowed_parents" gorm:"type:varchar(200)"` // CSV kode yang boleh jadi parent-nya
	IsSystem       bool   `json:"is_system"`
	Sort           int    `json:"sort"`
	IsActive       bool   `json:"is_active" gorm:"default:true"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EmploymentLevel — level organisasi utk track fungsional (kamus; admin bisa tambah).
type EmploymentLevel struct {
	Code      string `json:"code" gorm:"type:varchar(30);primaryKey"` // staff, supervisor, manager
	Label     string `json:"label" gorm:"type:varchar(100);not null"`
	AppliesTo string `json:"applies_to" gorm:"type:varchar(200)"` // CSV employee_type code yang relevan
	Sort      int    `json:"sort"`
	IsActive  bool   `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AcademicRank — jenjang akademik dosen (kamus; admin bisa tambah ubah urutan).
type AcademicRank struct {
	Code  string `json:"code" gorm:"type:varchar(30);primaryKey"` // asisten_ahli, lektor, ...
	Label string `json:"label" gorm:"type:varchar(100);not null"`
	Sort  int    `json:"sort"`
	IsActive bool `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// StructuralPosition — jabatan struktural (yang menjadikan pemegangnya approver di unit tsb).
type StructuralPosition struct {
	Code           string `json:"code" gorm:"type:varchar(30);primaryKey"` // kaprodi, dekan, kepala_unit, ...
	Label          string `json:"label" gorm:"type:varchar(100);not null"`
	ScopeUnitTypes string `json:"scope_unit_types" gorm:"type:varchar(200)"` // CSV org_unit_type tempat posisi ini berlaku
	IsApprovalHead bool   `json:"is_approval_head"`                          // pemegang posisi ini = kepala/approver unit
	IsSystem       bool   `json:"is_system"`
	Sort           int    `json:"sort"`
	IsActive       bool   `json:"is_active" gorm:"default:true"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// EmployeeUnitAssignment — afiliasi karyawan ke unit org (multi, satu primary).
// Dual affiliation (dosen+staff) = 2 baris. Jabatan struktural (kaprodi/dekan/kepala unit)
// tercatat di sini per unit — dasar routing approval.
type EmployeeUnitAssignment struct {
	ID                 string `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	EmployeeID         string `json:"employee_id" gorm:"type:varchar(36);index;not null"`
	UnitID             string `json:"unit_id" gorm:"type:varchar(36);index;not null"` // departments.id
	IsPrimary          bool   `json:"is_primary" gorm:"default:false"`
	StructuralPosition string `json:"structural_position,omitempty" gorm:"type:varchar(30)"` // kode structural_positions, kosong = anggota
	ValidFrom          *time.Time `json:"valid_from,omitempty"`
	ValidTo            *time.Time `json:"valid_to,omitempty"`
	CreatedBy          string `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
