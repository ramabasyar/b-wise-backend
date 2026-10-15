package entity

import "time"

// ===== Binawan Library — modul Kunjungan =====
// Skema mengikuti dokumen rencana 2026-10-15-Rencana-Pengembangan-Library-Kunjungan.md

// LibraryLocation — lokasi/cabang perpustakaan.
type LibraryLocation struct {
	Base
	Code     string `json:"code" gorm:"uniqueIndex;type:varchar(32);not null"`
	Name     string `json:"name" gorm:"type:varchar(128);not null"`
	Timezone string `json:"timezone" gorm:"type:varchar(64);not null;default:'Asia/Jakarta'"`
	IsActive bool   `json:"is_active" gorm:"not null;default:true"`
}

func (LibraryLocation) TableName() string { return "library_locations" }

// LibraryKiosk — perangkat kiosk check-in.
type LibraryKiosk struct {
	Base
	LocationID     string     `json:"location_id" gorm:"type:varchar(36);index"`
	Code           string     `json:"code" gorm:"uniqueIndex;type:varchar(32);not null"`
	Name           string     `json:"name" gorm:"type:varchar(128);not null"`
	CredentialHash string     `json:"-" gorm:"type:varchar(255)"`
	Status         string     `json:"status" gorm:"type:varchar(16);not null;default:'active'` // active|disabled
	LastSeenAt     *time.Time `json:"last_seen_at"`
	AppVersion     string     `json:"app_version" gorm:"type:varchar(32)"`
}

func (LibraryKiosk) TableName() string { return "library_kiosks" }

// LibraryMember — anggota perpustakaan (mahasiswa; siap diperluas ke pegawai).
type LibraryMember struct {
	Base
	MemberType         string `json:"member_type" gorm:"type:varchar(16);not null;default:'student'` // student|employee
	MemberNumber       string `json:"member_number" gorm:"uniqueIndex;type:varchar(32);not null"`    // NIM untuk mahasiswa
	DisplayName        string `json:"display_name" gorm:"type:varchar(128);not null"`
	FacultyCode        string `json:"faculty_code" gorm:"type:varchar(32)"`
	FacultyName        string `json:"faculty_name" gorm:"type:varchar(128)"`
	StudyProgramCode   string `json:"study_program_code" gorm:"type:varchar(32)"`
	StudyProgramName   string `json:"study_program_name" gorm:"type:varchar(128)"`
	Email              string `json:"email" gorm:"type:varchar(128)"`
	Status             string `json:"status" gorm:"type:varchar(16);not null;default:'active'`              // active|inactive
	RegistrationSource string `json:"registration_source" gorm:"type:varchar(16);not null;default:'manual'` // manual|csv_import
	RegisteredBy       string `json:"registered_by" gorm:"type:varchar(36)"`
	UpdatedBy          string `json:"updated_by" gorm:"type:varchar(36)"`
}

func (LibraryMember) TableName() string { return "library_members" }

// LibraryMemberCard — kartu anggota (NFC primary).
type LibraryMemberCard struct {
	Base
	MemberID            string     `json:"member_id" gorm:"type:varchar(36);index;not null"`
	CardType            string     `json:"card_type" gorm:"type:varchar(16);not null;default:'nfc'` // nfc|barcode|qr|rfid
	CardTechnology      string     `json:"card_technology" gorm:"type:varchar(32)"`
	ReaderMode          string     `json:"reader_mode" gorm:"type:varchar(16)"`             // hid|pcsc_bridge
	CredentialLookup    string     `json:"-" gorm:"uniqueIndex;type:varchar(128);not null"` // HMAC deterministik
	CredentialEncrypted string     `json:"-" gorm:"type:text"`                              // hanya bila perlu pemulihan
	CredentialLast4     string     `json:"credential_last4" gorm:"type:varchar(8)"`
	Status              string     `json:"status" gorm:"type:varchar(16);not null;default:'active'` // active|lost|revoked|replaced
	IssuedAt            time.Time  `json:"issued_at" gorm:"autoCreateTime"`
	RevokedAt           *time.Time `json:"revoked_at"`
	RevokeReason        string     `json:"revoke_reason" gorm:"type:varchar(255)"`
	RegisteredBy        string     `json:"registered_by" gorm:"type:varchar(36)"`
	RevokedBy           string     `json:"revoked_by" gorm:"type:varchar(36)"`
}

func (LibraryMemberCard) TableName() string { return "library_member_cards" }

// LibraryMemberImportBatch — batch impor CSV anggota.
type LibraryMemberImportBatch struct {
	Base
	FileName    string `json:"file_name" gorm:"type:varchar(255)"`
	Checksum    string `json:"checksum" gorm:"type:varchar(64)"`
	Status      string `json:"status" gorm:"type:varchar(16);not null;default:'previewed'` // previewed|processing|completed|failed
	Inserted    int    `json:"inserted"`
	Updated     int    `json:"updated"`
	Skipped     int    `json:"skipped"`
	Failed      int    `json:"failed"`
	ColumnMap   string `json:"column_map" gorm:"type:jsonb"`
	ErrorReport string `json:"error_report" gorm:"type:text"`
	UploadedBy  string `json:"uploaded_by" gorm:"type:varchar(36)"`
}

func (LibraryMemberImportBatch) TableName() string { return "library_member_import_batches" }

// LibraryGuest — tamu eksternal.
type LibraryGuest struct {
	Base
	DisplayName  string `json:"display_name" gorm:"type:varchar(128);not null"`
	Organization string `json:"organization" gorm:"type:varchar(128)"`
	Contact      string `json:"contact" gorm:"type:varchar(64)"`
}

func (LibraryGuest) TableName() string { return "library_guests" }

// LibraryGuestPass — barcode/QR pass tamu (token opaque).
type LibraryGuestPass struct {
	Base
	GuestID     string    `json:"guest_id" gorm:"type:varchar(36);index;not null"`
	TokenLookup string    `json:"-" gorm:"uniqueIndex;type:varchar(128);not null"` // hash/HMAC token
	TokenLast4  string    `json:"token_last4" gorm:"type:varchar(8)"`
	ValidFrom   time.Time `json:"valid_from"`
	ExpiresAt   time.Time `json:"expires_at"`
	MaxUses     int       `json:"max_uses"`
	UseCount    int       `json:"use_count"`
	Status      string    `json:"status" gorm:"type:varchar(16);not null;default:'active'` // active|used|expired|revoked
	IssuedBy    string    `json:"issued_by" gorm:"type:varchar(36)"`
	RevokedBy   string    `json:"revoked_by" gorm:"type:varchar(36)"`
}

func (LibraryGuestPass) TableName() string { return "library_guest_passes" }

// LibraryVisitPurpose — tujuan kunjungan.
type LibraryVisitPurpose struct {
	Base
	Code      string `json:"code" gorm:"uniqueIndex;type:varchar(32);not null"`
	Name      string `json:"name" gorm:"type:varchar(64);not null"`
	SortOrder int    `json:"sort_order"`
	IsDefault bool   `json:"is_default" gorm:"not null;default:false"`
	IsActive  bool   `json:"is_active" gorm:"not null;default:true"`
}

func (LibraryVisitPurpose) TableName() string { return "library_visit_purposes" }

// LibraryVisit — record kunjungan.
type LibraryVisit struct {
	Base
	LocationID           string     `json:"location_id" gorm:"type:varchar(36);index"`
	KioskID              string     `json:"kiosk_id" gorm:"type:varchar(36)"`
	VisitorType          string     `json:"visitor_type" gorm:"type:varchar(16);not null"` // student|employee|guest
	MemberID             string     `json:"member_id" gorm:"type:varchar(36);index"`
	GuestID              string     `json:"guest_id" gorm:"type:varchar(36);index"`
	PurposeID            string     `json:"purpose_id" gorm:"type:varchar(36)"`
	CheckInAt            time.Time  `json:"check_in_at" gorm:"not null"`
	CheckOutAt           *time.Time `json:"check_out_at"`
	EntryMethod          string     `json:"entry_method" gorm:"type:varchar(24);not null"` // nfc|member_number|guest_barcode|manual|offline_sync
	FallbackReason       string     `json:"fallback_reason" gorm:"type:varchar(32)"`       // forgot_card
	SnapshotName         string     `json:"snapshot_name" gorm:"type:varchar(128)"`
	SnapshotFaculty      string     `json:"snapshot_faculty" gorm:"type:varchar(128)"`
	SnapshotStudyProgram string     `json:"snapshot_study_program" gorm:"type:varchar(128)"`
	SnapshotUnit         string     `json:"snapshot_unit" gorm:"type:varchar(128)"`
	ClientEventID        string     `json:"client_event_id" gorm:"uniqueIndex;type:varchar(64)"`
	Status               string     `json:"status" gorm:"type:varchar(16);not null;default:'recorded'` // recorded|voided
	CreatedBy            string     `json:"created_by" gorm:"type:varchar(64)"`
	VoidedBy             string     `json:"voided_by" gorm:"type:varchar(36)"`
	VoidedAt             *time.Time `json:"voided_at"`
	VoidReason           string     `json:"void_reason" gorm:"type:varchar(255)"`
}

func (LibraryVisit) TableName() string { return "library_visits" }

// LibraryAuditLog — audit perubahan sensitif.
type LibraryAuditLog struct {
	Base
	ActorID      string `json:"actor_id" gorm:"type:varchar(64)"`
	ActorType    string `json:"actor_type" gorm:"type:varchar(16)"` // user|kiosk|system
	Action       string `json:"action" gorm:"type:varchar(64);not null"`
	ResourceType string `json:"resource_type" gorm:"type:varchar(64);not null"`
	ResourceID   string `json:"resource_id" gorm:"type:varchar(64);not null"`
	BeforeValue  string `json:"before_value" gorm:"type:text"`
	AfterValue   string `json:"after_value" gorm:"type:text"`
	Reason       string `json:"reason" gorm:"type:varchar(255)"`
	RequestID    string `json:"request_id" gorm:"type:varchar(64)"`
	IP           string `json:"ip" gorm:"type:varchar(45)"`
}

func (LibraryAuditLog) TableName() string { return "library_audit_logs" }
