package service

import (
	"strings"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
)

// writeAudit — best-effort; kegagalan audit tidak membatalkan operasi utama.
func writeAudit(a repository.AuditLogRepository, actor, action, rtype, rid, reason string) {
	log := &entity.LibraryAuditLog{
		ActorID: actor, Action: action, ResourceType: rtype, ResourceID: rid, Reason: reason,
	}
	if i := strings.Index(actor, ":"); i > 0 {
		log.ActorType = actor[:i]
	}
	_ = a.Create(log)
}
