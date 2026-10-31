package ticketsvc

import (
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ===== Basis Pengetahuan (KB, blueprint 6.9) =====

// KBListTeam - seluruh artikel tim user, termasuk draft (KT-09).
func (s *Service) KBListTeam(userID string) ([]entity.KBArticle, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var list []entity.KBArticle
	err = s.db.Where("team_id = ?", team.ID).Order("updated_at DESC").Find(&list).Error
	return list, err
}

// KBSearch - artikel TERBIT utk semua akun (KB-02): full-text konfigurasi
// indonesian; bila kueri kosong/gagal, jatuh ke ILIKE sederhana.
func (s *Service) KBSearch(q, teamCode string) ([]entity.KBArticle, error) {
	qs := strings.TrimSpace(q)
	scope := func() *gorm.DB {
		db := s.db.Model(&entity.KBArticle{}).Where("status = ?", "published")
		if teamCode != "" {
			db = db.Where("team_id = (SELECT id FROM teams WHERE code = ? AND is_active LIMIT 1)", teamCode)
		}
		if qs == "" {
			db = db.Order("updated_at DESC")
		}
		return db.Limit(20)
	}
	var list []entity.KBArticle
	if qs != "" {
		vec := "to_tsvector('indonesian', coalesce(title,'') || ' ' || coalesce(body,''))"
		err := scope().Where(vec+" @@ websearch_to_tsquery('indonesian', ?)", qs).
			Clauses(clause.OrderBy{Expression: clause.Expr{SQL: "ts_rank(" + vec + ", websearch_to_tsquery('indonesian', ?)) DESC", Vars: []interface{}{qs}}}).
			Order("updated_at DESC").
			Find(&list).Error
		if err == nil && len(list) > 0 {
			return list, nil
		}
		list = nil
		like := "%" + strings.ToLower(qs) + "%"
		return list, scope().Where("LOWER(title) LIKE ? OR LOWER(body) LIKE ?", like, like).Find(&list).Error
	}
	return list, scope().Find(&list).Error
}

// KBGetForViewer - terbit utk semua akun; draft hanya anggota tim pemilik (KB-01).
func (s *Service) KBGetForViewer(id, userID string) (*entity.KBArticle, error) {
	var a entity.KBArticle
	if err := s.db.Where("id = ?", id).First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	if a.Status != "published" && !s.IsMember(a.TeamID, userID) {
		return nil, ErrForbidden
	}
	return &a, nil
}

// KBCreate - draft artikel baru milik tim user.
func (s *Service) KBCreate(userID, title, body string) (*entity.KBArticle, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: judul artikel wajib", ErrValidation)
	}
	a := entity.KBArticle{Base: entity.Base{}, TeamID: team.ID, Slug: slugify(title),
		Title: strings.TrimSpace(title), Body: body, Status: "draft", Version: 1, Audience: "{}", CreatedBy: userID}
	a.Audience = jsonbOr(a.Audience)
	if err := s.db.Create(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// KBUpdate - ubah field whitelist; versi +1 tiap simpan; terbit mengisi published_at.
func (s *Service) KBUpdate(userID, id string, fields map[string]interface{}) (*entity.KBArticle, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var a entity.KBArticle
	if err := s.db.Where("id = ? AND team_id = ?", id, team.ID).First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	allowed := map[string]bool{"title": true, "body": true, "status": true, "audience": true}
	upd := map[string]interface{}{}
	for k, v := range fields {
		if allowed[k] {
			upd[k] = v
		}
	}
	if len(upd) == 0 {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", ErrValidation)
	}
	if st, ok := upd["status"].(string); ok {
		if st != "draft" && st != "published" && st != "archived" {
			return nil, fmt.Errorf("%w: status tidak dikenal", ErrValidation)
		}
		if st == "published" && a.Status != "published" {
			upd["published_at"] = s.now()
		}
	}
	if v, ok := upd["audience"].(string); ok {
		upd["audience"] = jsonbOr(v)
	}
	upd["version"] = a.Version + 1
	if err := s.db.Model(&a).Updates(upd).Error; err != nil {
		return nil, err
	}
	_ = s.db.Where("id = ?", id).First(&a).Error
	return &a, nil
}

// KBFeedback - membantu/tidak membantu, sekali per user, boleh diubah (KB-05).
func (s *Service) KBFeedback(userID, id string, helpful bool) (*entity.KBArticle, error) {
	var a entity.KBArticle
	if err := s.db.Where("id = ? AND status = ?", id, "published").First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	var fb entity.KBArticleFeedback
	err := s.db.Where("article_id = ? AND user_id = ?", id, userID).First(&fb).Error
	if err == nil {
		if fb.Helpful == helpful {
			return &a, nil
		}
		_ = s.db.Model(&fb).Update("helpful", helpful).Error
		if helpful {
			_ = s.db.Model(&a).Updates(map[string]interface{}{"helpful": gorm.Expr("helpful + 1"), "not_helpful": gorm.Expr("CASE WHEN not_helpful > 0 THEN not_helpful - 1 ELSE 0 END")}).Error
		} else {
			_ = s.db.Model(&a).Updates(map[string]interface{}{"not_helpful": gorm.Expr("not_helpful + 1"), "helpful": gorm.Expr("CASE WHEN helpful > 0 THEN helpful - 1 ELSE 0 END")}).Error
		}
	} else {
		fb = entity.KBArticleFeedback{Base: entity.Base{}, ArticleID: id, UserID: userID, Helpful: helpful}
		if err := s.db.Create(&fb).Error; err != nil {
			return nil, err
		}
		col := "not_helpful"
		if helpful {
			col = "helpful"
		}
		_ = s.db.Model(&a).Update(col, gorm.Expr(col+" + 1")).Error
	}
	_ = s.db.Where("id = ?", id).First(&a).Error
	return &a, nil
}

// KBFromTicket - jawaban publik tiket dijadikan draft artikel (KB-04).
func (s *Service) KBFromTicket(userID, ticketID, eventID, title string) (*entity.KBArticle, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	var ev entity.TicketEvent
	if err := s.db.Where("id = ? AND ticket_id = ?", eventID, ticketID).First(&ev).Error; err != nil {
		return nil, fmt.Errorf("%w: balasan tidak ditemukan", ErrNotFound)
	}
	if strings.TrimSpace(title) == "" {
		title = t.Subject
	}
	a := entity.KBArticle{Base: entity.Base{}, TeamID: t.TeamID, Slug: slugify(title),
		Title: strings.TrimSpace(title), Body: ev.Body, Status: "draft", Version: 1, Audience: "{}", CreatedBy: userID}
	a.Audience = jsonbOr(a.Audience)
	if err := s.db.Create(&a).Error; err != nil {
		return nil, err
	}
	_ = s.AddSystemEvent(ticketID, "jawaban tiket dijadikan draft artikel KB", ev.ID)
	return &a, nil
}
