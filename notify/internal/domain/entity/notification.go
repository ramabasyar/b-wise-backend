package entity

import "time"

// Outbox — antrean notifikasi keluar terpusat (fase-2, keputusan C).
// Kanal: email (SMTP), wa (webhook gateway), push (PWA web-push, PM-13).
type Outbox struct {
	Base
	DedupeKey     *string    `json:"-" gorm:"type:varchar(180);uniqueIndex"`
	Delivered     string     `json:"-" gorm:"type:jsonb;default:'[]'"`
	Channel       string     `json:"channel" gorm:"type:varchar(10);not null;index"` // email|wa|push
	Recipient     string     `json:"recipient" gorm:"type:varchar(255);not null"`    // alamat email / nomor / user_id (push)
	UserID        string     `json:"user_id,omitempty" gorm:"type:varchar(36);index"`
	Topic         string     `json:"topic" gorm:"type:varchar(60);not null"`
	Title         string     `json:"title" gorm:"type:varchar(200);not null"`
	Body          string     `json:"body" gorm:"type:text"`
	Payload       string     `json:"payload,omitempty" gorm:"type:jsonb"` // url dsb. — WAJIB jsonbOr()
	Status        string     `json:"status" gorm:"type:varchar(12);not null;default:'pending';index"`
	Attempts      int        `json:"attempts" gorm:"default:0"`
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty" gorm:"type:timestamptz"`
	SentAt        *time.Time `json:"sent_at,omitempty" gorm:"type:timestamptz"`
	Error         string     `json:"error,omitempty" gorm:"type:varchar(500)"`
}

func (Outbox) TableName() string { return "notifications_outbox" }

// PushSubscription — langganan PWA push per user (endpoint Web Push + kunci).
type PushSubscription struct {
	Base
	UserID    string `json:"user_id" gorm:"type:varchar(36);not null;index"`
	Endpoint  string `json:"endpoint" gorm:"type:varchar(500);not null;uniqueIndex"`
	P256DH    string `json:"p256dh" gorm:"type:varchar(255);not null"`
	Auth      string `json:"auth" gorm:"type:varchar(255);not null"`
	UserAgent string `json:"user_agent,omitempty" gorm:"type:varchar(300)"`
}

func (PushSubscription) TableName() string { return "push_subscriptions" }
