package ticketsvc

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Retensi & anonimisasi (AD-05, UU PDP) =====

// RunRetensi - pemindai harian: tiket lama yang sudah ditutup dianonimkan
// sesuai RETENSI_HARI (0/non-set = nonaktif).
func (s *Service) RunRetensi(ctx context.Context, every time.Duration) {
	if ctx == nil {
		ctx = context.Background()
	}
	if every <= 0 {
		every = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		s.scanRetensi()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.scanRetensi()
			}
		}
	}()
	log.Printf("[retensi] aktif (interval %s; RETENSI_HARI=%s)", every, os.Getenv("RETENSI_HARI"))
}

func (s *Service) scanRetensi() {
	days, _ := strconv.Atoi(os.Getenv("RETENSI_HARI"))
	if days <= 0 {
		return
	}
	cutoff := s.now().AddDate(0, 0, -days)
	res := s.db.Model(&entity.Ticket{}).
		Where("status = ? AND closed_at IS NOT NULL AND closed_at < ? AND requester_user_id <> ?", "ditutup", cutoff, "[retensi]").
		Updates(map[string]interface{}{"requester_user_id": "[retensi]", "nim": "", "answers": "{}", "on_behalf_by": ""})
	if res.RowsAffected > 0 {
		log.Printf("[retensi] %d tiket ditutup dianonimkan (lebih dari %d hari)", res.RowsAffected, days)
	}
}
