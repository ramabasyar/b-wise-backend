package service

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

// Sentinel errors — handler memetakan ke HTTP status.
var (
	ErrValidation     = errors.New("data tidak lengkap atau tidak valid")
	ErrNotFound       = errors.New("data tidak ditemukan")
	ErrMemberExists   = errors.New("NIM sudah terdaftar")
	ErrMemberInactive = errors.New("anggota tidak aktif")
	ErrCardTaken      = errors.New("kartu sudah terdaftar pada anggota lain")
	ErrCardNotActive  = errors.New("kartu tidak aktif")
	ErrHMACKeyMissing = errors.New("LIBRARY_HMAC_KEY belum dikonfigurasi")
	ErrPhotoRequired  = errors.New("foto mahasiswa wajib diambil dari webcam")
	ErrPhotoInvalid   = errors.New("format foto tidak valid")
)

const maxMemberPhotoBytes = 1024 * 1024

// MemberService — registry anggota & kartu NFC.
type MemberService struct {
	members repository.MemberRepository
	cards   repository.MemberCardRepository
	audits  repository.AuditLogRepository
	hmacKey string
}

func NewMemberService(m repository.MemberRepository, c repository.MemberCardRepository, a repository.AuditLogRepository, hmacKey string) *MemberService {
	return &MemberService{members: m, cards: c, audits: a, hmacKey: hmacKey}
}

type CreateMemberInput struct {
	MemberNumber     string `json:"member_number"`
	DisplayName      string `json:"display_name"`
	MemberType       string `json:"member_type"`
	FacultyCode      string `json:"faculty_code"`
	FacultyName      string `json:"faculty_name"`
	StudyProgramCode string `json:"study_program_code"`
	StudyProgramName string `json:"study_program_name"`
	Email            string `json:"email"`
	PhotoDataURL     string `json:"photo_data_url"`
}

func decodeMemberPhoto(dataURL string) ([]byte, string, error) {
	parts := strings.SplitN(strings.TrimSpace(dataURL), ",", 2)
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "data:image/jpeg;base64") {
		return nil, "", ErrPhotoInvalid
	}
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil || len(data) == 0 || len(data) > maxMemberPhotoBytes {
		return nil, "", ErrPhotoInvalid
	}
	if len(data) < 3 || data[0] != 0xff || data[1] != 0xd8 || data[2] != 0xff {
		return nil, "", ErrPhotoInvalid
	}
	return data, "image/jpeg", nil
}

func (s *MemberService) audit(actor, action, rtype, rid, reason string, before, after interface{}) {
	a := &entity.LibraryAuditLog{
		ActorID: actor, Action: action, ResourceType: rtype, ResourceID: rid, Reason: reason,
	}
	if i := strings.Index(actor, ":"); i > 0 {
		a.ActorType = actor[:i]
	}
	_ = s.audits.Create(a)
}

func (s *MemberService) Create(in CreateMemberInput, actor string) (*entity.LibraryMember, error) {
	in.MemberNumber = strings.TrimSpace(in.MemberNumber)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.MemberNumber == "" || in.DisplayName == "" {
		return nil, ErrValidation
	}
	if strings.TrimSpace(in.PhotoDataURL) == "" {
		return nil, ErrPhotoRequired
	}
	photo, photoType, err := decodeMemberPhoto(in.PhotoDataURL)
	if err != nil {
		return nil, err
	}
	if in.MemberType == "" {
		in.MemberType = "student"
	}
	if existing, _ := s.members.GetByNumber(in.MemberNumber); existing != nil {
		return nil, ErrMemberExists
	}
	m := &entity.LibraryMember{
		MemberType: in.MemberType, MemberNumber: in.MemberNumber, DisplayName: in.DisplayName,
		FacultyCode: in.FacultyCode, FacultyName: in.FacultyName,
		StudyProgramCode: in.StudyProgramCode, StudyProgramName: in.StudyProgramName,
		Email: in.Email, Photo: photo, PhotoContentType: photoType,
		Status: "active", RegistrationSource: "manual", RegisteredBy: actor,
	}
	if err := s.members.Create(m); err != nil {
		return nil, err
	}
	s.audit(actor, "member.create", "library_members", m.ID, "", nil, nil)
	return m, nil
}

func (s *MemberService) Update(id string, in CreateMemberInput, actor string) (*entity.LibraryMember, error) {
	m, err := s.members.GetByID(id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	if in.DisplayName != "" {
		m.DisplayName = strings.TrimSpace(in.DisplayName)
	}
	m.FacultyCode = pick(in.FacultyCode, m.FacultyCode)
	m.FacultyName = pick(in.FacultyName, m.FacultyName)
	m.StudyProgramCode = pick(in.StudyProgramCode, m.StudyProgramCode)
	m.StudyProgramName = pick(in.StudyProgramName, m.StudyProgramName)
	m.Email = pick(in.Email, m.Email)
	m.UpdatedBy = actor
	if err := s.members.Update(m); err != nil {
		return nil, err
	}
	s.audit(actor, "member.update", "library_members", m.ID, "", nil, nil)
	return m, nil
}

func pick(newVal, oldVal string) string {
	if newVal != "" {
		return newVal
	}
	return oldVal
}

func (s *MemberService) SetStatus(id, status, actor string) (*entity.LibraryMember, error) {
	m, err := s.members.GetByID(id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	m.Status = status
	m.UpdatedBy = actor
	if err := s.members.Update(m); err != nil {
		return nil, err
	}
	s.audit(actor, "member.status."+status, "library_members", m.ID, "", nil, nil)
	return m, nil
}

func (s *MemberService) Get(id string) (*entity.LibraryMember, error) {
	m, err := s.members.GetByID(id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	return m, nil
}

func (s *MemberService) List(f repository.MemberFilter) ([]entity.LibraryMember, int64, error) {
	return s.members.List(f)
}

type EnrollCardInput struct {
	Credential     string `json:"credential"`
	CardType       string `json:"card_type"`
	CardTechnology string `json:"card_technology"`
	ReaderMode     string `json:"reader_mode"`
}

// EnrollCard mendaftarkan/mengganti kartu anggota.
// Kebijakan: satu kartu aktif per anggota; kartu lama otomatis "replaced".
func (s *MemberService) EnrollCard(memberID string, in EnrollCardInput, actor string) (*entity.LibraryMemberCard, error) {
	if s.hmacKey == "" {
		return nil, ErrHMACKeyMissing
	}
	cred := credhash.Normalize(in.Credential)
	if len(cred) < 4 {
		return nil, ErrValidation
	}
	m, err := s.members.GetByID(memberID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, ErrNotFound
	}
	if m.Status != "active" {
		return nil, ErrMemberInactive
	}
	if in.CardType == "" {
		in.CardType = "nfc"
	}
	lookup := credhash.Lookup(s.hmacKey, cred)

	// Kartu dengan UID sama sudah ada di sistem:
	// - active/lost/revoked → tolak (dipakai anggota lain, atau kartu hilang yang dicabut).
	// - replaced (bekas salah-tap / ganti kartu) → alihkan ke pemilik baru;
	//   baris tetap satu per kartu fisik sehingga lookup unik terjaga.
	if existing, _ := s.cards.GetByLookup(lookup); existing != nil {
		if existing.Status != "replaced" {
			return nil, ErrCardTaken
		}
		if err := s.retireActiveCard(memberID, actor); err != nil {
			return nil, err
		}
		existing.MemberID = memberID
		existing.Status = "active"
		existing.RevokedAt = nil
		existing.RevokeReason = ""
		existing.RevokedBy = ""
		existing.RegisteredBy = actor
		if err := s.cards.Update(existing); err != nil {
			return nil, err
		}
		s.audit(actor, "card.reassigned", "library_member_cards", existing.ID, "reassign-salah-tap", nil, nil)
		return existing, nil
	}

	// Satu kartu aktif: tandai kartu lama replaced.
	if err := s.retireActiveCard(memberID, actor); err != nil {
		return nil, err
	}
	card := &entity.LibraryMemberCard{
		MemberID: memberID, CardType: in.CardType, CardTechnology: in.CardTechnology, ReaderMode: in.ReaderMode,
		CredentialLookup: lookup, CredentialLast4: credhash.Last4(cred),
		Status: "active", RegisteredBy: actor,
	}
	if err := s.cards.Create(card); err != nil {
		return nil, err
	}
	s.audit(actor, "card.enroll", "library_member_cards", card.ID, "", nil, nil)
	return card, nil
}

// retireActiveCard menandai kartu aktif anggota sebagai replaced (ganti kartu).
func (s *MemberService) retireActiveCard(memberID, actor string) error {
	old, err := s.cards.ActiveByMember(memberID)
	if err != nil || old == nil {
		return err
	}
	now := time.Now()
	old.Status = "replaced"
	old.RevokedAt = &now
	old.RevokeReason = "replacement"
	old.RevokedBy = actor
	if err := s.cards.Update(old); err != nil {
		return err
	}
	s.audit(actor, "card.replaced", "library_member_cards", old.ID, "replacement", nil, nil)
	return nil
}

// RevokeCard menonaktifkan kartu (lost|revoked|replaced) dengan alasan.
func (s *MemberService) RevokeCard(cardID, status, reason, actor string) (*entity.LibraryMemberCard, error) {
	if status != "lost" && status != "revoked" && status != "replaced" {
		return nil, ErrValidation
	}
	if status != "replaced" && strings.TrimSpace(reason) == "" {
		return nil, ErrValidation
	}
	c, err := s.cards.GetByID(cardID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, ErrNotFound
	}
	if c.Status != "active" {
		return nil, ErrCardNotActive
	}
	now := time.Now()
	c.Status = status
	c.RevokedAt = &now
	c.RevokeReason = reason
	c.RevokedBy = actor
	if err := s.cards.Update(c); err != nil {
		return nil, err
	}
	s.audit(actor, "card."+status, "library_member_cards", c.ID, reason, nil, nil)
	return c, nil
}

func (s *MemberService) CardHistory(memberID string) ([]entity.LibraryMemberCard, error) {
	return s.cards.ListByMember(memberID)
}
