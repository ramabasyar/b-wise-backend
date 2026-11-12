package ticketsvc

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// Lampiran tiket — disimpan di filesystem (object key relatif root),
// metadata di tabel ticket_attachments. Unduh selalu lewat service dengan
// cek otorisasi viewer tiket (pemohon atau anggota tim penanganan).

var (
	allowedAttachExt = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".pdf": true, ".docx": true}
	allowedAttachMime = map[string]bool{"image/jpeg": true, "image/png": true, "application/pdf": true, "application/vnd.openxmlformats-officedocument.wordprocessingml.document": true}
)

type AttachmentMeta struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Filename  string    `json:"filename"`
	Mime      string    `json:"mime,omitempty"`
	Size      int64     `json:"size"`
	Sha256    string    `json:"sha256,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func toMeta(a entity.TicketAttachment) AttachmentMeta {
	return AttachmentMeta{ID: a.ID, TicketID: a.TicketID, Filename: a.Filename, Mime: a.Mime, Size: a.Size, Sha256: a.Sha256, CreatedAt: a.CreatedAt}
}

// SetAttachmentLimits — pasang root penyimpanan & batas ukuran per berkas (dipanggil saat wiring).
func (s *Service) SetAttachmentLimits(dir string, maxBytes int64) {
	s.attachDir = dir
	s.attachMaxBytes = maxBytes
}

func (s *Service) attachmentDir() string {
	if s.attachDir != "" {
		return s.attachDir
	}
	return filepath.Join("data", "attachments")
}

// ListAttachments — metadata lampiran sebuah tiket (viewer tiket saja).
func (s *Service) ListAttachments(ticketID, userID string) ([]AttachmentMeta, error) {
	if _, err := s.GetForViewer(ticketID, userID); err != nil {
		return nil, err
	}
	var rows []entity.TicketAttachment
	if err := s.db.Where("ticket_id = ?", ticketID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]AttachmentMeta, 0, len(rows))
	for _, r := range rows {
		out = append(out, toMeta(r))
	}
	return out, nil
}

// SaveAttachment — simpan satu berkas untuk tiket (viewer tiket saja).
// Validasi ekstensi+mime (jpg/jpeg/png/pdf/docx) dan batas ukuran maxBytes.
func (s *Service) SaveAttachment(ticketID, userID, filename, contentType string, size int64, r io.Reader, maxBytes int64) (AttachmentMeta, error) {
	if _, err := s.GetForViewer(ticketID, userID); err != nil {
		return AttachmentMeta{}, err
	}
	if size <= 0 {
		return AttachmentMeta{}, fmt.Errorf("%w: berkas kosong", ErrValidation)
	}
	if maxBytes > 0 && size > maxBytes {
		return AttachmentMeta{}, fmt.Errorf("%w: ukuran berkas melebihi batas", ErrValidation)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if !allowedAttachExt[ext] {
		return AttachmentMeta{}, fmt.Errorf("%w: format berkas tidak didukung (JPG, JPEG, PNG, PDF, DOCX)", ErrValidation)
	}
	if contentType != "" {
		ct := contentType
		if i := strings.Index(ct, ";"); i >= 0 {
			ct = strings.TrimSpace(ct[:i])
		}
		if parsed, _, err := mime.ParseMediaType(ct); err == nil {
			ct = parsed
		}
		if !allowedAttachMime[ct] {
			return AttachmentMeta{}, fmt.Errorf("%w: tipe konten tidak didukung", ErrValidation)
		}
	}

	// Baca ke memori dibatasi maxBytes+1 (lampiran helpdesk kecil; sederhana & aman).
	limit := maxBytes
	if limit <= 0 {
		limit = s.attachMaxBytes
	}
	if limit <= 0 {
		limit = 10 << 20
	}
	buf, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return AttachmentMeta{}, err
	}
	if int64(len(buf)) > limit {
		return AttachmentMeta{}, fmt.Errorf("%w: ukuran berkas melebihi batas", ErrValidation)
	}
	sum := sha256.Sum256(buf)
	sha := hex.EncodeToString(sum[:])

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return AttachmentMeta{}, err
	}
	objectKey := filepath.Join(ticketID, hex.EncodeToString(raw)+ext)
	root := s.attachmentDir()
	dst := filepath.Join(root, objectKey)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return AttachmentMeta{}, err
	}
	if err := os.WriteFile(dst, buf, 0o640); err != nil {
		return AttachmentMeta{}, err
	}

	att := entity.TicketAttachment{
		TicketID:  ticketID,
		ObjectKey: objectKey,
		Filename: filename,
		Mime:     contentType,
		Size:     int64(len(buf)),
		Sha256:   sha,
	}
	if err := s.db.Create(&att).Error; err != nil {
		_ = os.Remove(dst)
		return AttachmentMeta{}, err
	}
	return toMeta(att), nil
}

// ErrAttachmentGone — metadata ada tapi berkas hilang dari penyimpanan.
var ErrAttachmentGone = errors.New("lampiran tidak ada di penyimpanan")

// OpenAttachment — buka berkas lampiran untuk diunduh (viewer tiket saja).
func (s *Service) OpenAttachment(ticketID, attID, userID string) (string, string, io.ReadSeekCloser, error) {
	if _, err := s.GetForViewer(ticketID, userID); err != nil {
		return "", "", nil, err
	}
	var att entity.TicketAttachment
	if err := s.db.Where("id = ? AND ticket_id = ?", attID, ticketID).First(&att).Error; err != nil {
		return "", "", nil, err
	}
	path := filepath.Join(s.attachmentDir(), att.ObjectKey)
	f, err := os.Open(path)
	if err != nil {
		return "", "", nil, ErrAttachmentGone
	}
	ct := att.Mime
	if ct == "" {
		ct = "application/octet-stream"
	}
	return att.Filename, ct, f, nil
}
