package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"time"

	"github.com/rama/b-wise/web-management/internal/domain/entity"
	"gorm.io/gorm"
)

// MaxLegacyBytes — batas khusus migrasi aset lama (web hero bisa >15MB).
const MaxLegacyBytes = 64 << 20 // 64 MB

// ImportLegacy — migrasi satu aset gambar lama dari repo web ke media library.
// Idempoten: file_key deterministik dari sha1(relPath); re-run mengembalikan
// aset yang sudah ada tanpa duplikat. Kedua nilai kembalian: aset + created.
func (s *MediaService) ImportLegacy(ctx context.Context, raw []byte, filename, contentType, relPath, alt, actor string) (*entity.MediaAsset, bool, error) {
	if s.mio == nil {
		return nil, false, fmt.Errorf("storage MinIO belum dikonfigurasi: %w", ErrInvalid)
	}
	if !allowedMIMEs[contentType] || contentType == "application/pdf" {
		return nil, false, fmt.Errorf("tipe %q tidak diizinkan (hanya gambar): %w", contentType, ErrInvalid)
	}
	if int64(len(raw)) > MaxLegacyBytes {
		return nil, false, fmt.Errorf("aset legacy melebihi 64MB: %w", ErrInvalid)
	}
	sum := sha1.Sum([]byte(relPath))
	objID := fmt.Sprintf("%x", sum)
	var exist entity.MediaAsset
	if err := s.db.Where("file_key LIKE ?", "bwm/legacy/"+objID+".%").First(&exist).Error; err == nil {
		return &exist, false, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, false, err
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false, fmt.Errorf("gambar tidak bisa dibaca: %w", ErrInvalid)
	}
	b := img.Bounds()
	ext := normalizeExt(filepath.Ext(filename), format)
	key := "bwm/legacy/" + objID + ext
	if err := s.mio.Put(ctx, key, bytes.NewReader(raw), int64(len(raw)), contentType); err != nil {
		return nil, false, fmt.Errorf("upload ke storage gagal: %w", err)
	}
	variants := map[string]entity.MediaVariant{}
	for _, t := range variantTargets {
		if b.Dx() <= t.Width {
			continue
		}
		v := resizeToWidth(img, t.Width)
		buf := &bytes.Buffer{}
		if err := jpeg.Encode(buf, v, &jpeg.Options{Quality: 82}); err != nil {
			return nil, false, err
		}
		vKey := fmt.Sprintf("%s_%s_%d.jpg", key[:len(key)-len(ext)], t.Name, t.Width)
		if err := s.mio.Put(ctx, vKey, buf, int64(buf.Len()), "image/jpeg"); err != nil {
			return nil, false, err
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
		return nil, false, err
	}
	_ = time.Now() // (time dipakai entity via hook gorm)
	return asset, true, nil
}
