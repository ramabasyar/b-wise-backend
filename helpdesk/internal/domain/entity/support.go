package entity

import "time"

// CannedResponse — template balasan per tim (AG-06).
type CannedResponse struct {
	Base
	TeamID string `json:"team_id" gorm:"type:varchar(36);index;not null"`
	Title  string `json:"title" gorm:"type:varchar(150);not null"`
	Body   string `json:"body" gorm:"type:text;not null"`
}

func (CannedResponse) TableName() string { return "canned_responses" }

// CSATResponse — survei 5 aspek skala 1-4 saat Selesai (PM-08, D-12).
type CSATResponse struct {
	Base
	TicketID     string `json:"ticket_id" gorm:"type:varchar(36);uniqueIndex;not null"`
	Reliability    int `json:"reliability"`
	Responsiveness int `json:"responsiveness"`
	Assurance      int `json:"assurance"`
	Empathy        int `json:"empathy"`
	Tangible       int `json:"tangible"`
	Comment      string `json:"comment,omitempty" gorm:"type:text"`
	SubmittedAt  time.Time `json:"submitted_at"`
}

func (CSATResponse) TableName() string { return "csat_responses" }

// NotificationOutbox — antrean pesan keluar dengan retry (§9.5/§9.6).
type NotificationOutbox struct {
	Base
	Channel      string `json:"channel" gorm:"type:varchar(20);not null"`
	Recipient    string `json:"recipient" gorm:"type:varchar(255);not null"`
	Template     string `json:"template" gorm:"type:varchar(50)"`
	Payload      string `json:"payload,omitempty" gorm:"type:jsonb"`
	Status       string `json:"status" gorm:"type:varchar(20);default:'pending';index"`
	Attempts     int    `json:"attempts" gorm:"default:0"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty" gorm:"type:timestamptz"`
}

func (NotificationOutbox) TableName() string { return "notifications_outbox" }
