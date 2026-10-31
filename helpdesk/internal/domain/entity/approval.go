package entity

import "time"

// TicketApproval — langkah persetujuan berurutan sebelum tiket dikerjakan (AP-01..AP-03).
type TicketApproval struct {
	Base
	TicketID    string     `json:"ticket_id" gorm:"type:varchar(36);index;not null"`
	Step        int        `json:"step" gorm:"not null"`
	Rule        string     `json:"rule" gorm:"type:varchar(30)"`
	ApproverID  string     `json:"approver_id,omitempty" gorm:"type:varchar(36);index"`
	Decision    string     `json:"decision,omitempty" gorm:"type:varchar(20)"`
	Comment     string     `json:"comment,omitempty" gorm:"type:text"`
	DecidedAt   *time.Time `json:"decided_at,omitempty" gorm:"type:timestamptz"`
	DueAt       *time.Time `json:"due_at,omitempty" gorm:"type:timestamptz"`
	Mode        string     `json:"mode,omitempty" gorm:"type:varchar(10)"`
	RemindedAt  *time.Time `json:"reminded_at,omitempty" gorm:"type:timestamptz"`
	EscalatedAt *time.Time `json:"escalated_at,omitempty" gorm:"type:timestamptz"`
	EscalatedTo string     `json:"escalated_to,omitempty" gorm:"type:varchar(36)"`
}

func (TicketApproval) TableName() string { return "ticket_approvals" }
