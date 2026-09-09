package entity

import "time"

// Notification — F7: notifikasi in-app + delivery channel (email/WA/log).
type Notification struct {
	ID        string     `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	UserID    string     `json:"user_id" gorm:"type:varchar(36);not null;index:idx_notif_user_status"`
	Channel   string     `json:"channel" gorm:"type:varchar(10);not null;default:'inapp'"`                              // inapp|email|wa|log
	Status    string     `json:"status" gorm:"type:varchar(12);not null;default:'pending';index:idx_notif_user_status"` // pending|sent|failed|read
	Topic     string     `json:"topic" gorm:"type:varchar(60);not null"`                                                // achievement.rejected | actionplan.overdue | regulation.activated | compliance.digest
	Title     string     `json:"title" gorm:"type:varchar(200);not null"`
	Body      string     `json:"body" gorm:"type:text"`
	RefType   string     `json:"ref_type,omitempty" gorm:"type:varchar(40)"`
	RefID     string     `json:"ref_id,omitempty" gorm:"type:varchar(36)"`
	Error     string     `json:"error,omitempty" gorm:"type:varchar(500)"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty" gorm:"type:timestamptz"`
	ReadAt    *time.Time `json:"read_at,omitempty" gorm:"type:timestamptz"`
}

func (Notification) TableName() string { return "notifications" }
