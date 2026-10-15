package service

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

// KioskAdminService — provisioning perangkat kiosk.
type KioskAdminService struct {
	kiosks repository.KioskRepository
	audits repository.AuditLogRepository
}

func NewKioskAdminService(k repository.KioskRepository, a repository.AuditLogRepository) *KioskAdminService {
	return &KioskAdminService{kiosks: k, audits: a}
}

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
