package service

import (
	"encoding/json"
	"testing"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

type rotatingKioskRepo struct {
	repository.KioskRepository
	kiosk *entity.LibraryKiosk
	saves int
}

func (r *rotatingKioskRepo) GetByID(id string) (*entity.LibraryKiosk, error) {
	if r.kiosk != nil && r.kiosk.ID == id {
		return r.kiosk, nil
	}
	return nil, nil
}

func (r *rotatingKioskRepo) Update(k *entity.LibraryKiosk) error {
	r.saves++
	r.kiosk = k
	return nil
}

type rotatingAuditRepo struct {
	repository.AuditLogRepository
	log *entity.LibraryAuditLog
}

func (r *rotatingAuditRepo) Create(log *entity.LibraryAuditLog) error {
	r.log = log
	return nil
}

func TestRotateKioskSecretInvalidatesOldSecret(t *testing.T) {
	oldSecret := "old-secret"
	repo := &rotatingKioskRepo{kiosk: &entity.LibraryKiosk{
		Base: entity.Base{ID: "kiosk-1"},
		Code: "pintu-depan", Status: "active",
		CredentialHash: credhash.HashSecret(oldSecret),
	}}
	audits := &rotatingAuditRepo{}
	svc := NewKioskAdminService(repo, nil, audits)

	out, err := svc.RotateSecret("kiosk-1", "user:petugas")
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if repo.saves != 1 || out.Secret == "" || out.Secret == oldSecret {
		t.Fatalf("secret baru tidak tersimpan dengan benar")
	}
	if repo.kiosk.CredentialHash != credhash.HashSecret(out.Secret) ||
		repo.kiosk.CredentialHash == credhash.HashSecret(oldSecret) {
		t.Fatalf("hash lama masih berlaku atau hash baru tidak sesuai")
	}
	if audits.log == nil || audits.log.Action != "kiosk.secret.rotate" {
		t.Fatalf("rotasi secret tidak tercatat di audit")
	}
	encoded, _ := json.Marshal(out.Kiosk)
	if string(encoded) == "" || json.Valid(encoded) == false {
		t.Fatalf("respons kiosk bukan JSON valid")
	}
	var fields map[string]any
	_ = json.Unmarshal(encoded, &fields)
	if _, exposed := fields["credential_hash"]; exposed {
		t.Fatalf("hash rahasia bocor di respons kiosk")
	}
}
