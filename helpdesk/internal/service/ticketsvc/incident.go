package ticketsvc

import (
	"fmt"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

// ===== Insiden massal (AG-12) =====

// MakeIncident - angkat tiket menjadi induk insiden massal (petugas tim pemilik).
func (s *Service) MakeIncident(ticketID, userID string) (*entity.Ticket, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	if t.Type == "incident" {
		return &t, nil
	}
	_ = s.db.Model(&t).Update("type", "incident").Error
	t.Type = "incident"
	_ = s.AddSystemEvent(ticketID, "diangkat menjadi insiden massal (AG-12) - tiket terkait akan ditutup bersama", "")
	return &t, nil
}

// AttachChild - kaitkan tiket lain sebagai anak insiden (petugas tim induk).
func (s *Service) AttachChild(parentID, childID, userID string) (*entity.Ticket, error) {
	var parent entity.Ticket
	if err := s.db.Where("id = ?", parentID).First(&parent).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(parent.TeamID, userID) {
		return nil, ErrForbidden
	}
	var child entity.Ticket
	if err := s.db.Where("id = ?", childID).First(&child).Error; err != nil {
		return nil, ErrNotFound
	}
	if child.ID == parent.ID {
		return nil, fmt.Errorf("%w: tiket tidak bisa dikaitkan ke dirinya", ErrValidation)
	}
	if child.Status == "ditutup" || child.Status == "ditolak" || child.Status == "dibatalkan" {
		return nil, fmt.Errorf("%w: tiket anak sudah berakhir", ErrValidation)
	}
	_ = s.db.Model(&child).Update("parent_id", parentID).Error
	_ = s.addEvent(child.ID, "system", "public", "", "ditautkan ke insiden massal "+parent.Number, "")
	_ = s.AddSystemEvent(parentID, "tiket anak ditautkan: "+child.Number, "")
	s.notifyInApp(child.RequesterUserID, "status_tiket", &child, "Tiket ditautkan ke insiden", "Tiket Anda menjadi bagian insiden massal dan akan mengikuti penanganannya.")
	_ = s.db.Where("id = ?", childID).First(&child).Error
	return &child, nil
}

// IncidentChildren - daftar tiket anak insiden (viewer tiket induk).
func (s *Service) IncidentChildren(parentID string) ([]entity.Ticket, error) {
	var list []entity.Ticket
	err := s.db.Where("parent_id = ?", parentID).Order("created_at ASC").Find(&list).Error
	return list, err
}

// cascadeCloseChildren - induk selesai/ditutup -> anak ditutup bersama (AG-12).
func (s *Service) cascadeCloseChildren(parent *entity.Ticket, actorID string) {
	var children []entity.Ticket
	s.db.Where("parent_id = ? AND status NOT IN ?", parent.ID, []string{"ditutup", "ditolak", "dibatalkan"}).Find(&children)
	now := s.now()
	for _, ch := range children {
		_ = s.db.Model(&ch).Updates(map[string]interface{}{"status": ticketstate.Ditutup, "closed_at": now}).Error
		_ = s.addEvent(ch.ID, "system", "public", actorID, "ditutup mengikuti insiden massal "+parent.Number, "")
		s.notifyInApp(ch.RequesterUserID, "status_tiket", &ch, "Tiket "+ch.Number+" ditutup", "Insiden massal yang memuat tiket Anda telah selesai ditangani.")
	}
}
