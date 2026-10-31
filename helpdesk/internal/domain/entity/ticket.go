package entity

import "time"

// Ticket — satu permintaan/laporan dari satu pemohon (blueprint §5.4, v1.1 tanpa tamu).
type Ticket struct {
	Base
	Number           string `json:"number" gorm:"type:varchar(20);uniqueIndex;not null"` // PL-YYYY-NNNNNN
	Type             string `json:"type" gorm:"type:varchar(20);default:'request'"`
	ServiceID        string `json:"service_id,omitempty" gorm:"type:varchar(36);index"`
	FormVersionID    string `json:"form_version_id,omitempty" gorm:"type:varchar(36)"`
	TeamID           string `json:"team_id" gorm:"type:varchar(36);index;not null"`
	AssigneeID       string `json:"assignee_id,omitempty" gorm:"type:varchar(36);index"`
	RequesterUserID  string `json:"requester_user_id" gorm:"type:varchar(36);index;not null"`
	OnBehalfBy       string `json:"on_behalf_by,omitempty" gorm:"type:varchar(36)"`
	Channel          string `json:"channel" gorm:"type:varchar(20);default:'portal'"`
	Subject          string `json:"subject" gorm:"type:varchar(255);not null"`
	Answers          string `json:"answers,omitempty" gorm:"type:jsonb"`
	Priority         string `json:"priority" gorm:"type:varchar(20);default:'normal'"`
	Status           string `json:"status" gorm:"type:varchar(30);default:'baru';index"`
	Confidentiality  string `json:"confidentiality" gorm:"type:varchar(20);default:'normal'"`
	RoomRef          string `json:"room_ref,omitempty" gorm:"type:varchar(100)"`
	NIM              string `json:"nim,omitempty" gorm:"type:varchar(30)"`
	RequesterUnitID  string `json:"requester_unit_id,omitempty" gorm:"type:varchar(36)"`
	ParentID         string `json:"parent_id,omitempty" gorm:"type:varchar(36);index"`
	FirstResponseAt  *time.Time `json:"first_response_at,omitempty" gorm:"type:timestamptz"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty" gorm:"type:timestamptz"`
	ClosedAt         *time.Time `json:"closed_at,omitempty" gorm:"type:timestamptz"`
	DueResponseAt    *time.Time `json:"due_response_at,omitempty" gorm:"type:timestamptz;index"`
	DueResolutionAt  *time.Time `json:"due_resolution_at,omitempty" gorm:"type:timestamptz;index"`
	PausedAt       *time.Time `json:"paused_at,omitempty" gorm:"type:timestamptz"` // SL-03
	SLAWarnedAt    *time.Time `json:"sla_warned_at,omitempty" gorm:"type:timestamptz"`
	SLABreachedAt  *time.Time `json:"sla_breached_at,omitempty" gorm:"type:timestamptz"`
	SLAEscalatedAt *time.Time `json:"sla_escalated_at,omitempty" gorm:"type:timestamptz"`
}

func (Ticket) TableName() string { return "tickets" }

// TicketEvent — timeline + jejak perubahan; append-only (blueprint §5.4).
type TicketEvent struct {
	Base
	TicketID   string `json:"ticket_id" gorm:"type:varchar(36);index;not null"`
	Kind       string `json:"kind" gorm:"type:varchar(20);not null"`
	Visibility string `json:"visibility" gorm:"type:varchar(10);default:'public'"`
	AuthorID   string `json:"author_id,omitempty" gorm:"type:varchar(36)"`
	Body       string `json:"body,omitempty" gorm:"type:text"`
	Meta       string `json:"meta,omitempty" gorm:"type:jsonb"`
}

func (TicketEvent) TableName() string { return "ticket_events" }

// TicketAttachment — berkas di bucket MinIO privat; unduh lewat service dengan cek otorisasi.
type TicketAttachment struct {
	Base
	TicketID  string `json:"ticket_id" gorm:"type:varchar(36);index;not null"`
	EventID   string `json:"event_id,omitempty" gorm:"type:varchar(36)"`
	ObjectKey string `json:"object_key" gorm:"type:varchar(300);not null"`
	Filename  string `json:"filename" gorm:"type:varchar(255);not null"`
	Mime      string `json:"mime,omitempty" gorm:"type:varchar(100)"`
	Size      int64  `json:"size" gorm:"default:0"`
	Sha256    string `json:"sha256,omitempty" gorm:"type:varchar(64)"`
}

func (TicketAttachment) TableName() string { return "ticket_attachments" }
