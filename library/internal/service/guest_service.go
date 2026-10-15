package service

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

// GuestService — tamu eksternal + barcode/QR pass opaque.
type GuestService struct {
	guests  repository.GuestRepository
	passes  repository.GuestPassRepository
	audits  repository.AuditLogRepository
	hmacKey string
}

func NewGuestService(g repository.GuestRepository, p repository.GuestPassRepository, a repository.AuditLogRepository, hmacKey string) *GuestService {
	return &GuestService{guests: g, passes: p, audits: a, hmacKey: hmacKey}
}

type CreateGuestInput struct {
	DisplayName  string `json:"display_name"`
	Organization string `json:"organization"`
	Contact      string `json:"contact"`
	PassTTLHours int    `json:"pass_ttl_hours"`
}

// PassIssued — token hanya ditampilkan SEKALI saat penerbitan.
type PassIssued struct {
	PassID    string    `json:"pass_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *GuestService) CreateWithPass(in CreateGuestInput, actor string) (*entity.LibraryGuest, *PassIssued, error) {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.DisplayName == "" {
		return nil, nil, ErrValidation
	}
	g := &entity.LibraryGuest{DisplayName: in.DisplayName, Organization: strings.TrimSpace(in.Organization), Contact: strings.TrimSpace(in.Contact)}
	if err := s.guests.Create(g); err != nil {
		return nil, nil, err
	}
	writeAudit(s.audits, actor, "guest.create", "library_guests", g.ID, "")
	pass, err := s.IssuePass(g.ID, in.PassTTLHours, actor)
	if err != nil {
		return g, nil, err
	}
	return g, pass, nil
}

func (s *GuestService) IssuePass(guestID string, ttlHours int, actor string) (*PassIssued, error) {
	if s.hmacKey == "" {
		return nil, ErrHMACKeyMissing
	}
	g, err := s.guests.GetByID(guestID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrNotFound
	}
	if ttlHours <= 0 {
		ttlHours = 12 // default: sampai akhir sesi hari itu
	}
	if ttlHours > 24*7 {
		ttlHours = 24 * 7
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	pass := &entity.LibraryGuestPass{
		GuestID:     guestID,
		TokenLookup: credhash.Lookup(s.hmacKey, token), // case-sensitive: tanpa normalize
		TokenLast4:  credhash.Last4(token),
		ValidFrom:   now,
		ExpiresAt:   now.Add(time.Duration(ttlHours) * time.Hour),
		Status:      "active",
		IssuedBy:    actor,
	}
	if err := s.passes.Create(pass); err != nil {
		return nil, err
	}
	writeAudit(s.audits, actor, "pass.issue", "library_guest_passes", pass.ID, "")
	return &PassIssued{PassID: pass.ID, Token: token, ExpiresAt: pass.ExpiresAt}, nil
}

func (s *GuestService) RevokePass(passID, actor string) error {
	p, err := s.passes.GetByID(passID)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrNotFound
	}
	if p.Status != "active" {
		return ErrCardNotActive // dipakai ulang: "tidak aktif"
	}
	p.Status = "revoked"
	p.RevokedBy = actor
	if err := s.passes.Update(p); err != nil {
		return err
	}
	writeAudit(s.audits, actor, "pass.revoke", "library_guest_passes", p.ID, "")
	return nil
}

func (s *GuestService) ListGuests(search string, page, limit int) ([]entity.LibraryGuest, int64, error) {
	return s.guests.List(search, page, limit)
}

func (s *GuestService) PassesByGuest(guestID string) ([]entity.LibraryGuestPass, error) {
	return s.passes.ListByGuest(guestID)
}
