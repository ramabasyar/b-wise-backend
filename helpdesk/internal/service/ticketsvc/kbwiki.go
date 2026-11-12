package ticketsvc

import (
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Fase 2 wiki: koleksi, sub-halaman, revisi =====

// KBCollectionsList - koleksi tim user; super: semua tim.
func (s *Service) KBCollectionsList(teamID string) ([]entity.KBCollection, error) {
	var list []entity.KBCollection
	q := s.db
	if teamID != "" {
		q = q.Where("team_id = ?", teamID)
	}
	err := q.Order("sort_order ASC, name ASC").Find(&list).Error
	return list, err
}

// KBCollectionCreate - koleksi baru milik tim user.
func (s *Service) KBCollectionCreate(userID, name, description string) (*entity.KBCollection, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: nama koleksi wajib", ErrValidation)
	}
	c := entity.KBCollection{TeamID: team.ID, Name: strings.TrimSpace(name), Slug: slugify(name), Description: description}
	if err := s.db.Create(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// KBCollectionUpdate - rename/deskripsi/sort (whitelist).
func (s *Service) KBCollectionUpdate(userID, id string, fields map[string]interface{}) (*entity.KBCollection, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var c entity.KBCollection
	if err := s.db.Where("id = ? AND team_id = ?", id, team.ID).First(&c).Error; err != nil {
		return nil, ErrNotFound
	}
	allowed := map[string]bool{"name": true, "description": true, "sort_order": true}
	upd := map[string]interface{}{}
	for k, v := range fields {
		if allowed[k] {
			upd[k] = v
		}
	}
	if len(upd) == 0 {
		return nil, fmt.Errorf("%w: tidak ada field yang diubah", ErrValidation)
	}
	if n, ok := upd["name"].(string); ok {
		if strings.TrimSpace(n) == "" {
			return nil, fmt.Errorf("%w: nama koleksi wajib", ErrValidation)
		}
		upd["slug"] = slugify(n)
	}
	if err := s.db.Model(&c).Updates(upd).Error; err != nil {
		return nil, err
	}
	_ = s.db.Where("id = ?", id).First(&c).Error
	return &c, nil
}

// KBCollectionDelete - hapus koleksi; artikel lepas (collection_id NULL).
func (s *Service) KBCollectionDelete(userID, id string) error {
	team, err := s.TeamOf(userID)
	if err != nil {
		return err
	}
	var c entity.KBCollection
	if err := s.db.Where("id = ? AND team_id = ?", id, team.ID).First(&c).Error; err != nil {
		return ErrNotFound
	}
	if err := s.db.Model(&entity.KBArticle{}).Where("collection_id = ?", id).Update("collection_id", nil).Error; err != nil {
		return err
	}
	return s.db.Delete(&c).Error
}

// kbTeamScope - teamID utk akses tulis: super (anggota virtual) pakai tim manapun milik artikel;
// implementasi sederhana: user biasa = TeamOf; super = tim FO.
func (s *Service) kbWriteTeam(userID string) (*entity.Team, error) {
	return s.TeamOf(userID)
}

// kbSnapshotRevision - simpan isi LAMA artikel sebelum diubah.
func (s *Service) kbSnapshotRevision(a *entity.KBArticle, editor string) {
	rev := entity.KBRevision{ArticleID: a.ID, Version: a.Version, Title: a.Title, Body: a.Body, EditedBy: editor}
	_ = s.db.Create(&rev).Error
}

// KBRevisionsList - riwayat revisi artikel (termasuk snapshot versi berjalan), terbaru dulu.
func (s *Service) KBRevisionsList(userID, articleID string) ([]entity.KBRevision, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var a entity.KBArticle
	if err := s.db.Where("id = ? AND team_id = ?", articleID, team.ID).First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	var revs []entity.KBRevision
	err = s.db.Where("article_id = ?", articleID).Order("version DESC").Find(&revs).Error
	return revs, err
}

// KBRestoreRevision - pulihkan isi versi lama sebagai VERSI BARU (tidak menimpa riwayat).
func (s *Service) KBRestoreRevision(userID, articleID string, version int) (*entity.KBArticle, error) {
	team, err := s.TeamOf(userID)
	if err != nil {
		return nil, err
	}
	var a entity.KBArticle
	if err := s.db.Where("id = ? AND team_id = ?", articleID, team.ID).First(&a).Error; err != nil {
		return nil, ErrNotFound
	}
	var rev entity.KBRevision
	if err := s.db.Where("article_id = ? AND version = ?", articleID, version).First(&rev).Error; err != nil {
		return nil, ErrNotFound
	}
	s.kbSnapshotRevision(&a, userID)
	a.Title = rev.Title
	a.Body = rev.Body
	a.Version = a.Version + 1
	if err := s.db.Save(&a).Error; err != nil {
		return nil, err
	}
	go s.KBEmbedOne(a.ID)
	return &a, nil
}