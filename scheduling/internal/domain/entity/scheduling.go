package entity

import "time"

// ==================== SCHEDULING MASTER DATA (F0) ====================
// Visi: kemandirian data universitas — master data akademik (ruang, MK, dosen,
// rombel) hidup di service ini dan dibaca sistem lain (doorlock, aplikasi
// akademik). Sumber input: import CSV/Excel / manual CRUD (siakad belum open API;
// adapter import dirancang digantikan sync API nanti tanpa ubah inti).

// Building — gedung. (Universitas kecil: 1 gedung dipakai bersama semua fakultas.)
type Building struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code      string    `json:"code" gorm:"type:varchar(30);uniqueIndex;not null"` // GDG-UTAMA
	Name      string    `json:"name" gorm:"type:varchar(150);not null"`
	Address   string    `json:"address,omitempty" gorm:"type:varchar(300)"`
	Notes     string    `json:"notes,omitempty" gorm:"type:text"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	Source    string    `json:"source" gorm:"type:varchar(20);default:'manual'"` // manual|import
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Building) TableName() string { return "buildings" }

// RoomType — kamus tipe ruang (dynamic, per kampus bisa beda).
// Room.Type menyimpan CODE tipe ini (backward compatible dgn data lama).
// Flags menentukan eligible ruang utk kategori MK di solver:
//   - for_theory  → boleh dipakai MK teori (jika offering tidak mem-pin tipe)
//   - for_practice → boleh dipakai MK praktikum
//   - keduanya false → hanya dipakai jika offering.room_type mem-pin persis
//     (mis. "office"/"other" — tidak pernah di-auto-assign solver).
type RoomType struct {
	ID          string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code        string    `json:"code" gorm:"type:varchar(30);uniqueIndex;not null"` // theory, lab, klinik, studio…
	Name        string    `json:"name" gorm:"type:varchar(150);not null"`           // "Laboratorium Komputer"
	Description string    `json:"description,omitempty" gorm:"type:varchar(300)"`
	ForTheory   bool      `json:"for_theory" gorm:"default:false"`
	ForPractice bool      `json:"for_practice" gorm:"default:false"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (RoomType) TableName() string { return "room_types" }

// FacilityType — kamus fasilitas ruang (dinamis).
// kind menentukan bentuk nilai di Room.Facilities (jsonb {code: value}):
//   "boolean" → true/false (ada/tidak, mis. proyektor)
//   "number"  → jumlah unit (mis. pc: 40, hospital_bed: 8)
type FacilityType struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code      string    `json:"code" gorm:"type:varchar(30);uniqueIndex;not null"` // projector, pc, ac…
	Name      string    `json:"name" gorm:"type:varchar(150);not null"`
	Kind      string    `json:"kind" gorm:"type:varchar(10);not null;default:'boolean'"` // boolean|number
	UnitLabel string    `json:"unit_label,omitempty" gorm:"type:varchar(30)"` // "unit", "bed"…
	SortOrder int       `json:"sort_order" gorm:"default:0"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (FacilityType) TableName() string { return "facility_types" }

// CourseType — kamus jenis mata kuliah dinamis.
// room_need menentukan kebutuhan ruang sesi di solver:
//   "theory"   → ruang dengan flag for_theory
//   "practice" → ruang dengan flag for_practice
//   "any"      → semua ruang eligible (for_theory ∪ for_practice)
//   "none"     → tanpa ruang fisik (online/kerja lapangan) — solver melewati
// Course.Type menyimpan CODE tipe ini (backward compatible).
type CourseType struct {
	ID          string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code        string    `json:"code" gorm:"type:varchar(30);uniqueIndex;not null"` // theory, practice, online, seminar…
	Name        string    `json:"name" gorm:"type:varchar(150);not null"`
	RoomNeed    string    `json:"room_need" gorm:"type:varchar(20);not null;default:'theory'"` // theory|practice|any|none
	Description string    `json:"description,omitempty" gorm:"type:varchar(300)"`
	SortOrder   int       `json:"sort_order" gorm:"default:0"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (CourseType) TableName() string { return "course_types" }

// SolveConfig — bobot soft constraint solver (singleton, code="default").
// Bobot 0 = soft constraint nonaktif. Di-read saat solve & di-pass ke sidecar.
type SolveConfig struct {
	ID              string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Code            string    `json:"code" gorm:"type:varchar(20);uniqueIndex;not null"` // "default"
	SpreadWeight    int       `json:"spread_weight" gorm:"default:5"`   // S1: sesi offering sama di hari sama
	RoomWasteWeight int       `json:"room_waste_weight" gorm:"default:2"` // S2: ruang boros per 20 kursi kosong
	LastSlotWeight  int       `json:"last_slot_weight" gorm:"default:1"`  // S3: sesi di slot terakhir hari
	Notes           string    `json:"notes,omitempty" gorm:"type:text"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (SolveConfig) TableName() string { return "solve_configs" }

// TimePolicy — kebijakan durasi 1 SKS (singleton, code="default").
// Standar umum (Permendikbud): teori 50 mnt/SKS, praktikum 170 mnt/SKS.
type TimePolicy struct {
	ID                string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	Code              string    `json:"code" gorm:"type:varchar(20);uniqueIndex;not null"`
	SksMinutesTheory  int       `json:"sks_minutes_theory" gorm:"default:50"`  // menit per 1 SKS teori
	SksMinutesPractice int      `json:"sks_minutes_practice" gorm:"default:170"` // menit per 1 SKS praktikum
	Notes             string    `json:"notes,omitempty" gorm:"type:text"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (TimePolicy) TableName() string { return "time_policies" }

// Room — ruang. faculty_id NULL = ruang KOMUNAL (dipakai bersama lintas
// fakultas/prodi — realita kampus kecil). Terisi = milik eksklusif fakultas.
type Room struct {
	ID         string         `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	BuildingID string         `json:"building_id" gorm:"type:varchar(36);index;not null"`
	Code       string         `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"` // R-301
	Name       string         `json:"name" gorm:"type:varchar(150)"`
	Type       string         `json:"type" gorm:"type:varchar(30);not null;default:'theory'"` // code dari tabel room_types
	Capacity   int            `json:"capacity" gorm:"default:0"`
	Floor      int            `json:"floor" gorm:"default:0"`
	FacultyID  *string        `json:"faculty_id,omitempty" gorm:"type:varchar(36);index"`     // NULL = komunal
	Facilities map[string]any `json:"facilities,omitempty" gorm:"type:jsonb;serializer:json"` // {pc:40, projector:true, ac:true}
	Notes      string         `json:"notes,omitempty" gorm:"type:text"`
	IsActive   bool           `json:"is_active" gorm:"default:true"`
	Source     string         `json:"source" gorm:"type:varchar(20);default:'manual'"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`

	Building *Building `json:"building,omitempty" gorm:"foreignKey:BuildingID"`
}

func (Room) TableName() string { return "rooms" }

// Term — semester/ tahun akademik.
type Term struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code      string    `json:"code" gorm:"type:varchar(20);uniqueIndex;not null"` // 2026-1
	Name      string    `json:"name" gorm:"type:varchar(100);not null"`            // Ganjil 2026/2027
	StartDate time.Time `json:"start_date" gorm:"type:date"`
	EndDate   time.Time `json:"end_date" gorm:"type:date"`
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Term) TableName() string { return "terms" }

// Course — mata kuliah (master kurikulum).
type Course struct {
	ID             string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code           string    `json:"code" gorm:"type:varchar(30);uniqueIndex;not null"` // NUR101
	Name           string    `json:"name" gorm:"type:varchar(200);not null"`
	Sks            int       `json:"sks" gorm:"default:2;not null"`
	Type           string    `json:"type" gorm:"type:varchar(20);default:'theory'"`    // code dari tabel course_types
	CurriculumYear string    `json:"curriculum_year,omitempty" gorm:"type:varchar(9)"` // 2024
	Notes          string    `json:"notes,omitempty" gorm:"type:text"`
	Source         string    `json:"source" gorm:"type:varchar(20);default:'manual'"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Course) TableName() string { return "courses" }

// Lecturer — dosen (praktisi = is_external; user_id opsional link ke SSO/HC).
type Lecturer struct {
	ID         string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code       string    `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"` // NIDN/NIK
	Name       string    `json:"name" gorm:"type:varchar(150);not null"`
	Email      string    `json:"email,omitempty" gorm:"type:varchar(150)"`
	Phone      string    `json:"phone,omitempty" gorm:"type:varchar(30)"`
	Rank       string    `json:"rank,omitempty" gorm:"type:varchar(50)"` // Lektor/Kepala/…
	FacultyID  *string   `json:"faculty_id,omitempty" gorm:"type:varchar(36);index"`
	IsExternal bool      `json:"is_external" gorm:"default:false"` // praktisi industri
	MaxLoadSks int       `json:"max_load_sks" gorm:"default:12"`   // batas beban per semester
	IsActive   bool      `json:"is_active" gorm:"default:true"`
	Source     string    `json:"source" gorm:"type:varchar(20);default:'manual'"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (Lecturer) TableName() string { return "lecturers" }

// ClassGroup — rombel/kelas.
type ClassGroup struct {
	ID          string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Code        string    `json:"code" gorm:"type:varchar(50);uniqueIndex;not null"` // TI-3A
	Name        string    `json:"name" gorm:"type:varchar(150)"`
	ProgramID   string    `json:"program_id,omitempty" gorm:"type:varchar(36);index"` // prodi (master lokal dulu)
	ProgramName string    `json:"program_name,omitempty" gorm:"type:varchar(150)"`
	FacultyID   *string   `json:"faculty_id,omitempty" gorm:"type:varchar(36);index"`
	CohortYear  int       `json:"cohort_year" gorm:"default:0"` // angkatan
	SizeEst     int       `json:"size_est" gorm:"default:0"`    // perkiraan jumlah mhs
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	Source      string    `json:"source" gorm:"type:varchar(20);default:'manual'"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ClassGroup) TableName() string { return "class_groups" }

// TimeSlot — template slot waktu (dipakai solver; hari 1=Senin..7=Minggu).
type TimeSlot struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Label     string    `json:"label" gorm:"type:varchar(50);not null"`     // Senin 07:30-09:10
	Day       int       `json:"day" gorm:"not null"`                        // 1..7
	StartTime string    `json:"start_time" gorm:"type:varchar(5);not null"` // "07:30"
	EndTime   string    `json:"end_time" gorm:"type:varchar(5);not null"`
	Order     int       `json:"order"` // urutan slot dalam hari
	IsActive  bool      `json:"is_active" gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TimeSlot) TableName() string { return "time_slots" }

// LecturerAvailability — ketersediaan dosen (dosen praktisi hanya hari tertentu, dsb).
// Mode: available (bisa) | blocked (tidak bisa).
type LecturerAvailability struct {
	ID         string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	LecturerID string    `json:"lecturer_id" gorm:"type:varchar(36);index;not null"`
	Day        int       `json:"day" gorm:"not null"`
	StartTime  string    `json:"start_time" gorm:"type:varchar(5)"`
	EndTime    string    `json:"end_time" gorm:"type:varchar(5)"`
	Mode       string    `json:"mode" gorm:"type:varchar(12);default:'blocked'"` // available|blocked
	Notes      string    `json:"notes,omitempty" gorm:"type:varchar(200)"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (LecturerAvailability) TableName() string { return "lecturer_availabilities" }

// Offering — penawaran MK pada term (inti input solver F1):
// MK × dosen pengampu × rombel × pola sesi.
type Offering struct {
	ID              string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	TermID          string    `json:"term_id" gorm:"type:varchar(36);index;not null"`
	CourseID        string    `json:"course_id" gorm:"type:varchar(36);index;not null"`
	LecturerID      string    `json:"lecturer_id" gorm:"type:varchar(36);index"` // primary pengampu
	ClassGroupID    string    `json:"class_group_id" gorm:"type:varchar(36);index;not null"`
	SksEffective    int       `json:"sks_effective" gorm:"default:2"`              // bisa beda dr master (remedial dsb)
	SessionsPerWeek int       `json:"sessions_per_week" gorm:"default:1"`          // meeting per minggu
	SessionSks      int       `json:"session_sks" gorm:"default:2"`                // SKS per sesi (2 SKS = 1 sesi 100m)
	PracticeSks     int       `json:"practice_sks" gorm:"default:0"`                // SKS praktikum/minggu (sesi blok terpisah, ruang for_practice)
	RoomType        string    `json:"room_type,omitempty" gorm:"type:varchar(30)"` // pin tipe ruang (menang atas room_need)
	Notes           string    `json:"notes,omitempty" gorm:"type:text"`
	IsActive        bool      `json:"is_active" gorm:"default:true"`
	Source          string    `json:"source" gorm:"type:varchar(20);default:'manual'"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	Term       *Term       `json:"term,omitempty" gorm:"foreignKey:TermID"`
	Course     *Course     `json:"course,omitempty" gorm:"foreignKey:CourseID"`
	Lecturer   *Lecturer   `json:"lecturer,omitempty" gorm:"foreignKey:LecturerID"`
	ClassGroup *ClassGroup `json:"class_group,omitempty" gorm:"foreignKey:ClassGroupID"`
}

func (Offering) TableName() string { return "offerings" }
