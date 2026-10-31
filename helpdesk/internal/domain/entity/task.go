package entity

import "time"

// TicketTask — sub-tugas lintas unit pada tiket induk (AG-09, SL-06; blueprint §5.4).
// Dibuat otomatis dari template layanan (KT-08) atau manual oleh petugas; tiap tugas
// punya target OLA sendiri berbasis kalender jam layanan tim pemilik tugas.
type TicketTask struct {
	Base
	TicketID   string     `json:"ticket_id" gorm:"type:varchar(36);index;not null"`
	TeamID     string     `json:"team_id" gorm:"type:varchar(36);index;not null"`
	AssigneeID string     `json:"assignee_id,omitempty" gorm:"type:varchar(36)"`
	Title      string     `json:"title" gorm:"type:varchar(200);not null"`
	Status     string     `json:"status" gorm:"type:varchar(20);default:'open';index"`
	OLADueAt   *time.Time `json:"ola_due_at,omitempty" gorm:"type:timestamptz"`
	Sort       int        `json:"sort" gorm:"default:0"`
	DependsOn  string     `json:"depends_on,omitempty" gorm:"type:varchar(36)"`
	DoneAt     *time.Time `json:"done_at,omitempty" gorm:"type:timestamptz"`
}

func (TicketTask) TableName() string { return "ticket_tasks" }
