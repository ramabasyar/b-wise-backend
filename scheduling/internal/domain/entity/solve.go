package entity

import "time"

// ==================== SOLVE JOB & TIMETABLE (F1) ====================

// SolveJob — proses penjadwalan (async). Progress ditulis runner; dibaca UI.
type SolveJob struct {
	ID         string         `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	TermID     string         `json:"term_id" gorm:"type:varchar(36);index;not null"`
	Status     string         `json:"status" gorm:"type:varchar(20);default:'queued';index"` // queued|modelling|solving|writing|done|failed|cancelled
	Progress   int            `json:"progress" gorm:"default:0"`
	Phase      string         `json:"phase,omitempty" gorm:"type:varchar(30)"`
	Message    string         `json:"message,omitempty" gorm:"type:text"`
	Stats      map[string]any `json:"stats,omitempty" gorm:"type:jsonb;serializer:json"`
	TimeLimit  int            `json:"time_limit_seconds" gorm:"default:60"`
	Elapsed    *float64       `json:"elapsed,omitempty"`
	StartedBy  string         `json:"started_by,omitempty" gorm:"type:varchar(100)"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty" gorm:"type:timestamptz"`

	Term *Term `json:"term,omitempty" gorm:"foreignKey:TermID"`
}

func (SolveJob) TableName() string { return "solve_jobs" }

// TimetableEntry — satu sesi terjadwal (hasil solve, draft). Publish = F3.
type TimetableEntry struct {
	ID         string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	JobID      string    `json:"job_id" gorm:"type:varchar(36);index"`
	TermID     string    `json:"term_id" gorm:"type:varchar(36);index;not null"`
	OfferingID string    `json:"offering_id" gorm:"type:varchar(36);index;not null"`
	SessionKey string    `json:"session_key" gorm:"type:varchar(80)"` // offeringID#n
	CourseCode string    `json:"course_code" gorm:"type:varchar(30)"`
	GroupID    string    `json:"group_id" gorm:"type:varchar(36)"`
	LecturerID string    `json:"lecturer_id,omitempty" gorm:"type:varchar(36)"`
	SlotID     string    `json:"slot_id" gorm:"type:varchar(36);not null"`
	RoomID     string    `json:"room_id" gorm:"type:varchar(36);not null"`
	Day        int       `json:"day" gorm:"not null"`
	SlotOrder  int       `json:"slot_order"`
	StartTime  string    `json:"start_time" gorm:"type:varchar(5)"`
	EndTime    string    `json:"end_time" gorm:"type:varchar(5)"`
	Locked     bool      `json:"locked" gorm:"default:false"`                        // F2: kunci manual saat re-solve
	VersionID  *string   `json:"version_id,omitempty" gorm:"type:varchar(36);index"` // F3: terisi = bagian versi published (immutable); NULL = draft aktif
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Offering   *Offering   `json:"offering,omitempty" gorm:"foreignKey:OfferingID"`
	Room       *Room       `json:"room,omitempty" gorm:"foreignKey:RoomID"`
	ClassGroup *ClassGroup `json:"class_group,omitempty" gorm:"foreignKey:GroupID"`
}

func (TimetableEntry) TableName() string { return "timetable_entries" }

// TimetableVersion — snapshot jadwal yang dipublish (F3). Immutable setelah publish;
// sistem lain (doorlock, aplikasi akademik) membaca versi published, bukan draft.
type TimetableVersion struct {
	ID           string     `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	TermID       string     `json:"term_id" gorm:"type:varchar(36);index;not null"`
	JobID        string     `json:"job_id" gorm:"type:varchar(36)"`
	Name         string     `json:"name" gorm:"type:varchar(150)"`                      // "Jadwal Ganjil 2026/2027 — Final"
	Status       string     `json:"status" gorm:"type:varchar(20);default:'published'"` // published|archived
	EntriesCount int        `json:"entries_count"`
	Note         string     `json:"note,omitempty" gorm:"type:text"`
	PublishedBy  string     `json:"published_by,omitempty" gorm:"type:varchar(100)"`
	PublishedAt  *time.Time `json:"published_at,omitempty" gorm:"type:timestamptz"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (TimetableVersion) TableName() string { return "timetable_versions" }

// CalendarToken — token akses feed ICS tanpa header auth (Google Calendar "From URL"
// tidak bisa kirim Authorization header). Scope: room|group|lecturer; revocable.
type CalendarToken struct {
	ID        string    `json:"id" gorm:"primaryKey;type:varchar(36);default:gen_random_uuid()"`
	Token     string    `json:"token" gorm:"type:varchar(64);uniqueIndex;not null"`
	Scope     string    `json:"scope" gorm:"type:varchar(20);not null"` // room|group|lecturer
	ScopeID   string    `json:"scope_id" gorm:"type:varchar(36);not null"`
	Label     string    `json:"label,omitempty" gorm:"type:varchar(150)"`
	Revoked   bool      `json:"revoked" gorm:"default:false"`
	CreatedAt time.Time `json:"created_at"`
}

func (CalendarToken) TableName() string { return "calendar_tokens" }
