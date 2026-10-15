package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
)

var ErrVisitVoided = errors.New("kunjungan sudah dibatalkan")

// VisitService — riwayat, input manual, koreksi, dan pembatalan kunjungan.
type VisitService struct {
	visits   repository.VisitRepository
	members  repository.MemberRepository
	purposes repository.VisitPurposeRepository
	audits   repository.AuditLogRepository
}

func NewVisitService(v repository.VisitRepository, m repository.MemberRepository, p repository.VisitPurposeRepository, a repository.AuditLogRepository) *VisitService {
	return &VisitService{visits: v, members: m, purposes: p, audits: a}
}

func (s *VisitService) List(f repository.VisitFilter) ([]entity.LibraryVisit, int64, error) {
	return s.visits.List(f)
}

func (s *VisitService) Get(id string) (*entity.LibraryVisit, error) {
	v, err := s.visits.GetByID(id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNotFound
	}
	return v, nil
}

type ManualVisitInput struct {
	MemberNumber string `json:"member_number"`
	PurposeCode  string `json:"purpose_code"`
	Reason       string `json:"reason"`
}

// ManualCreate — operator mencatat kunjungan tanpa kiosk.
func (s *VisitService) ManualCreate(in ManualVisitInput, actor, locationID string) (*entity.LibraryVisit, error) {
	nim := strings.TrimSpace(in.MemberNumber)
	if nim == "" {
		return nil, ErrValidation
	}
	member, err := s.members.GetByNumber(nim)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, ErrNotFound
	}
	if member.Status != "active" {
		return nil, ErrMemberInactive
	}
	purposeCode := strings.TrimSpace(in.PurposeCode)
	if purposeCode == "" {
		purposeCode = "general"
	}
	purpose, _ := s.purposes.GetByCode(purposeCode)

	v := &entity.LibraryVisit{
		LocationID:           locationID,
		VisitorType:          member.MemberType,
		MemberID:             member.ID,
		CheckInAt:            time.Now(),
		EntryMethod:          "manual",
		Status:               "recorded",
		CreatedBy:            actor,
		SnapshotName:         member.DisplayName,
		SnapshotFaculty:      member.FacultyName,
		SnapshotStudyProgram: member.StudyProgramName,
	}
	if purpose != nil {
		v.PurposeID = purpose.ID
	}
	if err := s.visits.Create(v); err != nil {
		return nil, err
	}
	writeAudit(s.audits, actor, "visit.manual", "library_visits", v.ID, in.Reason)
	return v, nil
}

type CorrectVisitInput struct {
	PurposeCode          string `json:"purpose_code"`
	SnapshotName         string `json:"snapshot_name"`
	SnapshotFaculty      string `json:"snapshot_faculty"`
	SnapshotStudyProgram string `json:"snapshot_study_program"`
	SnapshotUnit         string `json:"snapshot_unit"`
}

// Correct — koreksi atribut kunjungan; before/after masuk audit.
func (s *VisitService) Correct(id string, in CorrectVisitInput, actor string) (*entity.LibraryVisit, error) {
	v, err := s.visits.GetByID(id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNotFound
	}
	if v.Status == "voided" {
		return nil, ErrVisitVoided
	}
	before, _ := json.Marshal(map[string]string{
		"purpose_code": v.PurposeID, "name": v.SnapshotName, "faculty": v.SnapshotFaculty,
		"study_program": v.SnapshotStudyProgram, "unit": v.SnapshotUnit,
	})
	if code := strings.TrimSpace(in.PurposeCode); code != "" {
		if p, _ := s.purposes.GetByCode(code); p != nil {
			v.PurposeID = p.ID
		}
	}
	v.SnapshotName = pick(in.SnapshotName, v.SnapshotName)
	v.SnapshotFaculty = pick(in.SnapshotFaculty, v.SnapshotFaculty)
	v.SnapshotStudyProgram = pick(in.SnapshotStudyProgram, v.SnapshotStudyProgram)
	v.SnapshotUnit = pick(in.SnapshotUnit, v.SnapshotUnit)
	if err := s.visits.Update(v); err != nil {
		return nil, err
	}
	after, _ := json.Marshal(map[string]string{
		"purpose_code": v.PurposeID, "name": v.SnapshotName, "faculty": v.SnapshotFaculty,
		"study_program": v.SnapshotStudyProgram, "unit": v.SnapshotUnit,
	})
	log := &entity.LibraryAuditLog{
		ActorID: actor, Action: "visit.correct", ResourceType: "library_visits", ResourceID: v.ID,
		BeforeValue: string(before), AfterValue: string(after),
	}
	_ = s.audits.Create(log)
	return v, nil
}

// Void — batalkan kunjungan dengan alasan wajib; record tidak dihapus.
func (s *VisitService) Void(id, reason, actor string) (*entity.LibraryVisit, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, ErrValidation
	}
	v, err := s.visits.GetByID(id)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNotFound
	}
	if v.Status == "voided" {
		return nil, ErrVisitVoided
	}
	now := time.Now()
	v.Status = "voided"
	v.VoidedAt = &now
	v.VoidReason = reason
	v.VoidedBy = actor
	if err := s.visits.Update(v); err != nil {
		return nil, err
	}
	writeAudit(s.audits, actor, "visit.void", "library_visits", v.ID, reason)
	return v, nil
}
