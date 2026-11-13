package ticketsvc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/adapter/storage"
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

// SetKBStorage - pasang object storage MinIO utk gambar KB (nil = filesystem).
func (s *Service) SetKBStorage(m *storage.MinIO) { s.kbStore = m }

func ctSafe(ct string) string {
	if ct == "" {
		return "application/octet-stream"
	}
	return ct
}

// KBImageUpload - simpan gambar artikel (jpg/png/gif/webp, maks 5MB); kembalikan URL publik.

var kbImageMime = map[string]bool{"image/jpeg": true, "image/png": true, "image/gif": true, "image/webp": true}

func (s *Service) KBImageUpload(userID, filename, contentType string, size int64, r io.Reader) (*entity.KBImage, error) {

	team, err := s.TeamOf(userID)

	if err != nil { return nil, err }

	if size <= 0 || size > 5<<20 {

		return nil, fmt.Errorf("%w: ukuran gambar 1KB-5MB", ErrValidation)

	}

	if !kbImageMime[contentType] {

		return nil, fmt.Errorf("%w: hanya JPG/PNG/GIF/WEBP", ErrValidation)

	}

	buf, err := io.ReadAll(io.LimitReader(r, 5<<20+1))

	if err != nil { return nil, err }

	if int64(len(buf)) > 5<<20 {

		return nil, fmt.Errorf("%w: ukuran gambar 1KB-5MB", ErrValidation)

	}

	img := entity.KBImage{TeamID: team.ID, ObjectKey: "", Filename: filename, Mime: contentType, Size: int64(len(buf))}

	if err := s.db.Create(&img).Error; err != nil { return nil, err }

	objectKey := "kb/" + img.ID + ".bin"

	if s.kbStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.kbStore.Put(ctx, objectKey, bytes.NewReader(buf), int64(len(buf)), contentType); err != nil {
			_ = s.db.Delete(&img).Error
			return nil, err
		}
	} else {
		dst := filepath.Join(s.attachmentDir(), objectKey)
		if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil { return nil, err }
		if err := os.WriteFile(dst, buf, 0o640); err != nil { return nil, err }
	}

	img.ObjectKey = objectKey

	if err := s.db.Model(&img).Update("object_key", objectKey).Error; err != nil { return nil, err }

	return &img, nil

}

// KBImageServe - stream gambar utk ditampilkan di artikel (publik by id).

func (s *Service) KBImageServe(id string) (string, string, io.ReadSeekCloser, error) {

	var img entity.KBImage

	if err := s.db.Where("id = ?", id).First(&img).Error; err != nil {

		return "", "", nil, ErrNotFound

	}

	if s.kbStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		obj, gerr := s.kbStore.Get(ctx, img.ObjectKey)
		cancel()
		if gerr == nil {
			return img.Filename, ctSafe(img.Mime), obj, nil
		}
		// fallback filesystem utk baris lama pra-MinIO
	}

	f, err := os.Open(filepath.Join(s.attachmentDir(), img.ObjectKey))

	if err != nil {

		return "", "", nil, fmt.Errorf("gambar tidak ada di penyimpanan")

	}

	return img.Filename, ctSafe(img.Mime), f, nil

}
