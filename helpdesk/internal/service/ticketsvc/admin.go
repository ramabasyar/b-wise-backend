package ticketsvc

import (
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Layanan (KT-04) =====

// MyServices - seluruh layanan tim user (termasuk draft).
func (s *Service) MyServices(userID string) ([]entity.Service, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var list []entity.Service
	err = s.db.Where("team_id = ?", team.ID).Order("name ASC").Find(&list).Error
	return list, err
}

func slugify(name string) string {
	sl := strings.ToLower(strings.TrimSpace(name))
	sl = strings.ReplaceAll(sl, " ", "-")
	for _, c := range []string{"/", ":", "?", "#", "&", "=", "+"} {
		sl = strings.ReplaceAll(sl, c, "")
	}
	return sl
}

// CreateService - layanan baru milik tim user (admin unit).
func (s *Service) CreateService(userID string, p entity.Service) (*entity.Service, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	if p.Name == "" {
		return nil, fmt.Errorf("%w: nama layanan wajib", ErrValidation)
	}
	if p.Slug == "" {
		p.Slug = slugify(p.Name)
	}
	if p.TicketType == "" {
		p.TicketType = "request"
	}
	if p.Status == "" {
		p.Status = "draft"
	}
	if p.Confidentiality == "" {
		p.Confidentiality = "normal"
	}
	p.TeamID = team.ID
	p.Audience = jsonbOr(p.Audience)
	p.ApprovalFlow = jsonbOr(p.ApprovalFlow)
	p.TaskTemplate = jsonbOr(p.TaskTemplate)
	if err := s.db.Create(&p).Error; err != nil {
		return nil, fmt.Errorf("%w: slug sudah dipakai?", ErrValidation)
	}
	return &p, nil
}

// UpdateService - ubah layanan milik tim user (field whitelist).
func (s *Service) UpdateService(userID, id string, fields map[string]interface{}) (*entity.Service, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var svc entity.Service
	if err := s.db.Where("id = ? AND team_id = ?", id, team.ID).First(&svc).Error; err != nil {
		return nil, ErrNotFound
	}
	allowed := map[string]bool{"name": true, "description": true, "requirements": true, "procedure": true, "fee_note": true, "output_note": true, "ticket_type": true, "status": true, "sla_policy_id": true, "approval_flow": true, "category_id": true, "confidentiality": true}
	upd := map[string]interface{}{}
	for k, v := range fields {
		if allowed[k] {
			upd[k] = v
		}
	}
	for _, k := range []string{"approval_flow"} {
		if v, ok := upd[k].(string); ok {
			upd[k] = jsonbOr(v)
		}
	}
	if len(upd) == 0 {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", ErrValidation)
	}
	if err := s.db.Model(&svc).Updates(upd).Error; err != nil {
		return nil, err
	}
	_ = s.db.Where("id = ?", id).First(&svc).Error
	return &svc, nil
}

// ===== Kategori =====

func (s *Service) Categories() ([]entity.ServiceCategory, error) {
	var list []entity.ServiceCategory
	err := s.db.Order("sort ASC, name ASC").Find(&list).Error
	return list, err
}

func (s *Service) CreateCategory(name, icon string, sort int) (*entity.ServiceCategory, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: nama kategori wajib", ErrValidation)
	}
	c := entity.ServiceCategory{Base: entity.Base{}, Name: name, Icon: icon, Sort: sort}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// ===== Anggota tim (KT-03) =====

func (s *Service) AddMember(teamID, actorID, userID, role string) (*entity.TeamMember, error) {
	if !s.IsMember(teamID, actorID) {
		return nil, ErrForbidden
	}
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id wajib", ErrValidation)
	}
	if role == "" {
		role = "agent"
	}
	var exist entity.TeamMember
	if err := s.db.Where("team_id = ? AND user_id = ?", teamID, userID).First(&exist).Error; err == nil {
		_ = s.db.Model(&exist).Updates(map[string]interface{}{"is_active": true, "role": role}).Error
		return &exist, nil
	}
	m := entity.TeamMember{Base: entity.Base{}, TeamID: teamID, UserID: userID, Role: role, IsActive: true, CreatedBy: actorID}
	if err := s.db.Create(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *Service) RemoveMember(teamID, actorID, userID string) error {
	if !s.IsMember(teamID, actorID) {
		return ErrForbidden
	}
	return s.db.Where("team_id = ? AND user_id = ?", teamID, userID).Update("is_active", false).Error
}

// ===== Klasifikasi front office (FO-02) =====

// Classify - FO menetapkan layanan: set service_id + pindah ke tim pemilik layanan.
func (s *Service) Classify(ticketID, actorID, serviceID string) (*entity.Ticket, error) {
	var svc entity.Service
	if err := s.db.Where("id = ? AND status = ?", serviceID, "published").First(&svc).Error; err != nil {
		return nil, fmt.Errorf("%w: layanan tidak tersedia", ErrNotFound)
	}
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	_ = s.db.Model(&t).Updates(map[string]interface{}{"service_id": svc.ID, "team_id": svc.TeamID}).Error
	_ = s.addEvent(ticketID, "transfer", "public", actorID, "klasifikasi layanan: "+svc.Name, "")
	s.notifyInApp(t.RequesterUserID, "status_tiket", &t, "Tiket "+t.Number+" diteruskan", "Tiket Anda diteruskan ke unit penanggung jawab layanan.")
	return &t, nil
}
