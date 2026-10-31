package entity

// TicketSequence — penomoran atomik tiket per tahun (PL-YYYY-NNNNNN).
type TicketSequence struct {
	Year int   `gorm:"primaryKey"`
	Last int64 `gorm:"not null;default:0"`
}

func (TicketSequence) TableName() string { return "ticket_sequences" }
