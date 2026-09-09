package entity

import "time"

// AccessRequest — pengajuan akses (Fase B). Approve → AssignRoleToUser.
// Approver di-resolve dari HC Org API (kepala unit yang mapping service-nya).
type AccessRequest struct {
	ID           string     `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	UserID       string     `json:"user_id" gorm:"type:varchar(36);index;not null"` // SSO user id penerima
	RequestedBy  string     `json:"requested_by" gorm:"type:varchar(36);index;not null"` // SSO user id pengaju (self/sponsored)
	RoleID       string     `json:"role_id" gorm:"type:varchar(36);index;not null"`
	Justification string    `json:"justification" gorm:"type:text"`
	Status       string     `json:"status" gorm:"type:varchar(20);index;default:pending"` // pending|approved|rejected|cancelled
	ApproverID   *string    `json:"approver_id,omitempty" gorm:"type:varchar(36);index"`  // resolved approver (SSO id)
	DecidedBy    *string    `json:"decided_by,omitempty" gorm:"type:varchar(36)"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	Notes        string     `json:"notes,omitempty" gorm:"type:text"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
