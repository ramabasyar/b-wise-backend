package entity

// TeamMember — keanggotaan petugas tim (sumber kebenaran peran petugas; blueprint §5.4).
// Peran: agent (petugas), supervisor (koordinator), admin (admin unit).
type TeamMember struct {
	Base
	TeamID    string `json:"team_id" gorm:"type:varchar(36);index;not null"`
	UserID    string `json:"user_id" gorm:"type:varchar(36);index;not null"`
	Role      string `json:"role" gorm:"type:varchar(20);default:'agent'"`
	IsActive  bool   `json:"is_active" gorm:"default:true;index"`
	CreatedBy string `json:"created_by,omitempty" gorm:"type:varchar(36)"`
}

func (TeamMember) TableName() string { return "team_members" }
