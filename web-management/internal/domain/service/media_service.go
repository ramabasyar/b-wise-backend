package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/gif"  // decoder registrasi
	_ "image/png"  // decoder registrasi
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "golang.org/x/image/webp" // decoder registrasi (baca webp utk resize)
	"golang.org/x/image/draw"
	"gorm.io/gorm"

	"github.com/rama/b-wise/web-management/internal/adapter/storage"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

// MediaService — upload gambar ke MinIO + varian ukuran otomatis + CRUD metadata.
type MediaService struct {
	db  *gorm.DB
	mio *storage.MinIO
}

func NewMediaService(db *gorm.DB, mio *storage.MinIO) *MediaService {
	return &MediaService{db: db, mio: mio}
}

const MaxUploadBytes = 15 << 20 // 15 MB

var allowedMIMEs = map[string]bool{
	"image/jpeg": true, "image/png": true, "image/webp": true, "image/gif": true,
}

// variantTargets — varian lebar tetap (JPEG q82; encode WebP menyusul di F1
// bersama benchmark libwebp — nilai "right-sized" sudah diraih varian JPEG).
var variantTargets = []struct {
	Name  string
	Width int
}{
	{"thumb", 320}, {"medium", 768}, {"large", 1280},
}

var fnameRe = regexp.MustCompile(`[^a-z0-9.-]+`)

func newObjectID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeExt(ext, format string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return strings.ToLower(ext)
	}
	switch format {
	case "jpeg":
		return ".jpg"
	case "png", "webp", "gif":
		return "." + format
	}
	return ".bin"
}

// Upload — validasi, decode, varian, simpan MinIO + record MediaAsset.
// Layout key: bwm/2006/01/<uuid>.<ext> (+ <uuid>_thumb_320.jpg dst utk varian).
func (s *MediaService) Upload(ctx context.Context, r io.Reader, filename, contentType string, size int64, alt string, actor string) (*entity.MediaAsset, error) {
	if s.mio == nil {
		return nil, fmt.Errorf("storage MinIO belum dikonfigurasi: %w", ErrInvalid)
	}
	if !allowedMIMEs[contentType] {
		return nil, fmt.Errorf("tipe %q tidak diizinkan (jpeg/png/webp/gif): %w", contentType, ErrInvalid)
	}
	raw, err := io.ReadAll(io.LimitReader(r, MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > MaxUploadBytes {
		return nil, fmt.Errorf("ukuran melebihi 15MB: %w", ErrInvalid)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("gambar tidak bisa dibaca: %w", ErrInvalid)
	}
	b := img.Bounds()

	id := newObjectID()
	prefix := "bwm/" + time.Now().Format("2006/01")
	ext := normalizeExt(filepath.Ext(filename), format)
	key := prefix + "/" + id + ext
	if err := s.mio.Put(ctx, key, bytes.NewReader(raw), int64(len(raw)), contentType); err != nil {
		return nil, fmt.Errorf("upload ke storage gagal: %w", err)
	}

	variants := map[string]entity.MediaVariant{}
	for _, t := range variantTargets {
		if b.Dx() <= t.Width {
			continue // jangan upscale — konsumen pakai original
		}
		v := resizeToWidth(img, t.Width)
		buf := &bytes.Buffer{}
		if err := jpeg.Encode(buf, v, &jpeg.Options{Quality: 82}); err != nil {
			return nil, err
		}
		vKey := fmt.Sprintf("%s/%s_%s_%d.jpg", prefix, id, t.Name, t.Width)
		if err := s.mio.Put(ctx, vKey, buf, int64(buf.Len()), "image/jpeg"); err != nil {
			return nil, err
		}
		variants[t.Name] = entity.MediaVariant{
			Key: vKey, URL: s.mio.PublicURL(vKey),
			Width: v.Bounds().Dx(), Height: v.Bounds().Dy(),
		}
	}
	vJSON := "{}"
	if vb, err := json.Marshal(variants); err == nil {
		vJSON = string(vb)
	}

	asset := &entity.MediaAsset{
		FileKey: key, URL: s.mio.PublicURL(key), Filename: filepath.Base(filename), MIME: contentType,
		Width: b.Dx(), Height: b.Dy(), SizeBytes: int64(len(raw)),
		Variants: vJSON, Alt: alt, UploadedBy: actor,
	}
	if err := s.db.Create(asset).Error; err != nil {
		return nil, err
	}
	return asset, nil
}

// resizeToWidth — downscale proporsional (CatmullRom, kualitas tinggi).
func resizeToWidth(src image.Image, targetW int) image.Image {
	b := src.Bounds()
	if b.Dx() <= targetW {
		return src
	}
	targetH := int(float64(b.Dy()) * float64(targetW) / float64(b.Dx()))
	if targetH < 1 {
		targetH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// ==================== CRUD ====================

func (s *MediaService) List(q string, page, per int) ([]entity.MediaAsset, int64, error) {
	tx := s.db.Model(&entity.MediaAsset{})
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("lower(filename) LIKE ? OR lower(alt) LIKE ?", like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 24
	}
	var rows []entity.MediaAsset
	err := tx.Order("created_at DESC").Limit(per).Offset((page - 1) * per).Find(&rows).Error
	return rows, total, err
}

func (s *MediaService) Get(id string) (*entity.MediaAsset, error) {
	var m entity.MediaAsset
	if err := s.db.First(&m, "id = ?", id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// Delete — hapus objek (original + varian) dari MinIO lalu record DB.
func (s *MediaService) Delete(ctx context.Context, id string) error {
	m, err := s.Get(id)
	if err != nil {
		return err
	}
	if s.mio != nil {
		_ = s.mio.Delete(ctx, m.FileKey)
		for _, v := range m.VariantsMap() {
			if v.Key != "" {
				_ = s.mio.Delete(ctx, v.Key)
			}
		}
	}
	if err := s.db.Delete(m).Error; err != nil {
		return err
	}
	return nil
}

// UpdateMeta — perbarui alt/caption/credit tanpa menyentuh file.
func (s *MediaService) UpdateMeta(id, alt, caption, credit string) (*entity.MediaAsset, error) {
	m, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(m).Updates(map[string]any{
		"alt": alt, "caption": caption, "credit": credit,
	}).Error; err != nil {
		return nil, err
	}
	return s.Get(id)
}

// ==================== F2: MEDIA LANJUT (usage tracking + replace) ====================

// Usage — di mana aset dipakai: posts (hero/blocks), banners, events.
func (s *MediaService) Usage(id string) (map[string]any, error) {
	var m entity.MediaAsset
	if err := s.db.First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	like := "%" + m.URL + "%"
	var posts []entity.Post
	if err := s.db.Where("hero_media_id = ? OR hero_url = ? OR lower(translations::text) LIKE lower(?)", m.ID, m.URL, like).
		Order("updated_at DESC").Limit(200).Find(&posts).Error; err != nil {
		return nil, err
	}
	type postRef struct {
		ID, Slug, Status, Title string
	}
	pout := make([]postRef, 0, len(posts))
	for i := range posts {
		tr := entity.TrOf[entity.PostTranslation](posts[i].Translations)["id"]
		pout = append(pout, postRef{posts[i].ID, posts[i].Slug, posts[i].Status, tr.Title})
	}
	var banners []entity.Banner
	if err := s.db.Where("image_media_id = ? OR image_url = ?", m.ID, m.URL).Find(&banners).Error; err != nil {
		return nil, err
	}
	var events []entity.Event
	if err := s.db.Where("hero_media_id = ? OR hero_url = ?", m.ID, m.URL).Find(&events).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"posts": pout, "banners": banners, "events": events,
	}, nil
}

// Replace — ganti FILE aset: key & URL TIDAK berubah (semua referensi konten tetap
// aman); varian di-regen di key yang sama (overwrite).
func (s *MediaService) Replace(ctx context.Context, id string, r io.Reader, filename, contentType string, actor string) (*entity.MediaAsset, error) {
	if s.mio == nil {
		return nil, fmt.Errorf("storage MinIO belum dikonfigurasi: %w", ErrInvalid)
	}
	if !allowedMIMEs[contentType] {
		return nil, fmt.Errorf("tipe %q tidak diizinkan (jpeg/png/webp/gif): %w", contentType, ErrInvalid)
	}
	var m entity.MediaAsset
	if err := s.db.First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > MaxUploadBytes {
		return nil, fmt.Errorf("ukuran melebihi 15MB: %w", ErrInvalid)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("gambar tidak bisa dibaca: %w", ErrInvalid)
	}
	b := img.Bounds()

	// overwrite object utama — key & URL tetap
	if err := s.mio.Put(ctx, m.FileKey, bytes.NewReader(raw), int64(len(raw)), contentType); err != nil {
		return nil, fmt.Errorf("overwrite storage gagal: %w", err)
	}

	// regen varian di key yang sama (prefix + id dari FileKey lama)
	prefix := filepath.Dir(m.FileKey)
	objID := strings.TrimSuffix(filepath.Base(m.FileKey), filepath.Ext(m.FileKey))
	variants := map[string]entity.MediaVariant{}
	for _, t := range variantTargets {
		if b.Dx() <= t.Width {
			continue
		}
		v := resizeToWidth(img, t.Width)
		buf := &bytes.Buffer{}
		if err := jpeg.Encode(buf, v, &jpeg.Options{Quality: 82}); err != nil {
			return nil, err
		}
		vKey := fmt.Sprintf("%s/%s_%s_%d.jpg", prefix, objID, t.Name, t.Width)
		if err := s.mio.Put(ctx, vKey, buf, int64(buf.Len()), "image/jpeg"); err != nil {
			return nil, err
		}
		variants[t.Name] = entity.MediaVariant{
			Key: vKey, URL: s.mio.PublicURL(vKey),
			Width: v.Bounds().Dx(), Height: v.Bounds().Dy(),
		}
	}
	vJSON := "{}"
	if vb, err := json.Marshal(variants); err == nil {
		vJSON = string(vb)
	}

	m.MIME = contentType
	m.Width = b.Dx()
	m.Height = b.Dy()
	m.SizeBytes = int64(len(raw))
	m.Variants = vJSON
	m.UploadedBy = actor
	if err := s.db.Save(&m).Error; err != nil {
		return nil, err
	}
	return &m, nil
}
