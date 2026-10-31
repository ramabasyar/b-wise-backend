package ticketsvc

import (
	"encoding/json"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// notifyInApp - tulis notifikasi in-app ke outbox (PM-07; blueprint 9.6).
// Status langsung sent: in-app siap tampil; kanal email diproses worker notifsvc.
func (s *Service) notifyInApp(recipient, template string, t *entity.Ticket, title, body string) {
	if recipient == "" || t == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{
		"ticket_id": t.ID,
		"number":    t.Number,
		"title":     title,
		"body":      body,
	})
	row := entity.NotificationOutbox{
		Base:      entity.Base{},
		Channel:   "in_app",
		Recipient: recipient,
		Template:  template,
		Payload:   string(payload),
		Status:    "sent",
	}
	_ = s.db.Create(&row).Error
}

// Notifications - daftar notifikasi in-app user (terbaru dulu) + jumlah belum dibaca.
func (s *Service) Notifications(userID string) ([]entity.NotificationOutbox, int64, error) {
	var list []entity.NotificationOutbox
	err := s.db.Where("channel = ? AND recipient = ?", "in_app", userID).Order("created_at DESC").Limit(30).Find(&list).Error
	var unread int64
	s.db.Model(&entity.NotificationOutbox{}).Where("channel = ? AND recipient = ? AND status = ?", "in_app", userID, "sent").Count(&unread)
	return list, unread, err
}

// NotificationsRead - tandai notifikasi dibaca (ids kosong = semua milik user).
func (s *Service) NotificationsRead(userID string, ids []string) error {
	q := s.db.Model(&entity.NotificationOutbox{}).Where("channel = ? AND recipient = ? AND status = ?", "in_app", userID, "sent")
	if len(ids) > 0 {
		q = q.Where("id IN ?", ids)
	}
	return q.Update("status", "read").Error
}
