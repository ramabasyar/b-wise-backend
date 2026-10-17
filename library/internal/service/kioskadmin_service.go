package service

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

// KioskAdminService — provisioning perangkat kiosk.
type KioskAdminService struct {
	kiosks repository.KioskRepository
	visits repository.VisitRepository
	audits repository.AuditLogRepository
}

func NewKioskAdminService(k repository.KioskRepository, v repository.VisitRepository, a repository.AuditLogRepository) *KioskAdminService {
	return &KioskAdminService{kiosks: k, visits: v, audits: a}
}

// ErrKioskHasVisits — kiosk dengan riwayat kunjungan tidak boleh dihapus
// (kunjungan akan kehilangan referensi perangkat); nonaktifkan saja.
var ErrKioskHasVisits = errors.New("kiosk sudah memiliki riwayat kunjungan — nonaktifkan alih-alih hapus")

// KioskProvisioned — secret hanya ditampilkan SEKALI saat provisioning.
type KioskProvisioned struct {
	Kiosk  *entity.LibraryKiosk `json:"kiosk"`
	Secret string               `json:"secret"`
}

func (s *KioskAdminService) Provision(code, name, locationID, actor string) (*KioskProvisioned, error) {
	code = strings.TrimSpace(code)
	if code == "" || strings.TrimSpace(name) == "" {
		return nil, ErrValidation
	}
	if existing, _ := s.kiosks.GetByCode(code); existing != nil {
		return nil, ErrMemberExists // dipakai ulang semantic: "sudah terdaftar"
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	k := &entity.LibraryKiosk{
		Code:           code,
		Name:           strings.TrimSpace(name),
		LocationID:     locationID,
		CredentialHash: credhash.HashSecret(secret),
		Status:         "active",
	}
	if err := s.kiosks.Create(k); err != nil {
		return nil, err
	}
	writeAudit(s.audits, actor, "kiosk.provision", "library_kiosks", k.ID, "")
	return &KioskProvisioned{Kiosk: k, Secret: secret}, nil
}

func (s *KioskAdminService) List() ([]entity.LibraryKiosk, error) {
	return s.kiosks.List()
}

// SetStatus mengaktifkan/menonaktifkan kiosk. Kiosk nonaktif langsung ditolak
// oleh KioskAuth (403) — cocok untuk perangkat rusak/diganti sambil menjaga riwayat.
func (s *KioskAdminService) SetStatus(id, status, actor string) (*entity.LibraryKiosk, error) {
	if status != "active" && status != "disabled" {
		return nil, ErrValidation
	}
	k, err := s.kiosks.GetByID(id)
	if err != nil {
		return nil, err
	}
	if k == nil {
		return nil, ErrNotFound
	}
	if k.Status == status {
		return k, nil
	}
	k.Status = status
	if err := s.kiosks.Update(k); err != nil {
		return nil, err
	}
	writeAudit(s.audits, actor, "kiosk."+status, "library_kiosks", k.ID, "")
	return k, nil
}

// Delete menghapus kiosk permanen — hanya bila BELUM punya kunjungan tercatat.
func (s *KioskAdminService) Delete(id, actor string) error {
	k, err := s.kiosks.GetByID(id)
	if err != nil {
		return err
	}
	if k == nil {
		return ErrNotFound
	}
	n, err := s.visits.CountByKiosk(id)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrKioskHasVisits
	}
	if err := s.kiosks.Delete(id); err != nil {
		return err
	}
	writeAudit(s.audits, actor, "kiosk.delete", "library_kiosks", id, "")
	return nil
}
