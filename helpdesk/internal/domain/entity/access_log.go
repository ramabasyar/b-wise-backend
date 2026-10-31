package entity

// TicketAccessLog - jejak audit pembukaan tiket rahasia (RH-06).
type TicketAccessLog struct {
	Base
	TicketID string `json:"ticket_id" gorm:"type:varchar(36);index;not null"`
	UserID   string `json:"user_id" gorm:"type:varchar(36);index"`
	Action   string `json:"action" gorm:"type:varchar(30)"` // view | stage | track
	Detail   string `json:"detail,omitempty" gorm:"type:varchar(200)"`
}

func (TicketAccessLog) TableName() string { return "ticket_access_logs" }
