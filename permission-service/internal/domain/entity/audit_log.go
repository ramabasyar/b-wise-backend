package entity

import "time"

// AuditLog merekam setiap mutasi assignment (grant/revoke access, permission, role).
// Sumber kebenaran "siapa memberi apa kepada siapa kapan" untuk keperluan compliance.
type AuditLog struct {
	ID         string    `json:"id" gorm:"type:varchar(36);primary_key;default:gen_random_uuid()"`
	ActorID    string    `json:"actor_id" gorm:"type:varchar(64);index"`  // siapa yang melakukan (user/system)
	Action     string    `json:"action" gorm:"type:varchar(64);index"`   // grant_access, revoke_access, grant_permission, revoke_permission, assign_role, revoke_role
	UserID     string    `json:"user_id" gorm:"type:varchar(64);index"`  // target user
	ServiceID  string    `json:"service_id,omitempty" gorm:"type:varchar(36);index"`
	ObjectType string    `json:"object_type" gorm:"type:varchar(32)"`    // access | permission | role
	ObjectID   string    `json:"object_id,omitempty" gorm:"type:varchar(64)"`
	Detail     string    `json:"detail,omitempty" gorm:"type:jsonb"`     // JSON bebas (nama role, nama permission, dll)
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
