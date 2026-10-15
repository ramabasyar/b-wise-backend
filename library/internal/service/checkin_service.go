package service

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
	"github.com/rama/b-wise/library/internal/pkg/credhash"
)

var (
	ErrUnknownCredential = errors.New("kartu tidak dikenal")
	ErrRateLimited       = errors.New("terlalu banyak percobaan NIM, coba lagi beberapa saat")
	ErrPassInvalid       = errors.New("pass tamu tidak valid atau kedaluwarsa")
	ErrBadMethod         = errors.New("metode check-in tidak didukung")
)

// DedupWindow — jendela scan ulang yang tidak membuat kunjungan baru.
const DedupWindow = 10 * time.Minute

// CheckinService — mesin check-in kiosk (NFC, NIM fallback, guest barcode).
type CheckinService struct {
	members   repository.MemberRepository
	cards     repository.MemberCardRepository
	guests    repository.GuestRepository
	passes    repository.GuestPassRepository
	visits    repository.VisitRepository
	purposes  repository.VisitPurposeRepository
	locations repository.LocationRepository
	audits    repository.AuditLogRepository
	hmacKey   string
	limiter   *nimLimiter
}

func NewCheckinService(
	m repository.MemberRepository, mc repository.MemberCardRepository,
	g repository.GuestRepository, gp repository.GuestPassRepository,
	v repository.VisitRepository, p repository.VisitPurposeRepository,
	l repository.LocationRepository, a repository.AuditLogRepository,
	hmacKey string,
) *CheckinService {
	return &CheckinService{
		members: m, cards: mc, guests: g, passes: gp,
		visits: v, purposes: p, locations: l, audits: a,
		hmacKey: hmacKey, limiter: newNimLimiter(),
	}
}

type CheckinInput struct {
	Method        string `json:"method"` // nfc | member_number | guest_barcode
	Credential    string `json:"credential"`
	PurposeCode   string `json:"purpose_code"`
	ClientEventID string `json:"client_event_id"`
}

type CheckinResult struct {
	VisitID      string `json:"visit_id"`
	VisitorName  string `json:"visitor_name"`
	VisitorType  string `json:"visitor_type"`
	StudyProgram string `json:"study_program,omitempty"`
	Faculty      string `json:"faculty,omitempty"`
	Purpose      string `json:"purpose,omitempty"`
	Duplicate    bool   `json:"duplicate"`
}

// CheckIn mencatat kunjungan: idempotent per client_event_id,
// dedup 10 menit per orang/lokasi, fallback NIM di-rate-limit.
func (s *CheckinService) CheckIn(in CheckinInput, kiosk *entity.LibraryKiosk) (*CheckinResult, error) {
	in.Method = strings.TrimSpace(in.Method)
	switch in.Method {
	case "nfc", "member_number", "guest_barcode":
	default:
		return nil, ErrBadMethod
	}

	// 1. Idempotency kiosk (retry jaringan).
	if in.ClientEventID != "" {
		if existing, _ := s.visits.GetByClientEventID(in.ClientEventID); existing != nil {
			return s.resultFromVisit(existing, true), nil
		}
	}

	// 2. Tujuan kunjungan (default general).
	purposeCode := strings.TrimSpace(in.PurposeCode)
	if purposeCode == "" {
		purposeCode = "general"
	}
	purpose, _ := s.purposes.GetByCode(purposeCode)

	// 3. Lokasi: dari kiosk, atau default.
	locationID := ""
	kioskActor := ""
	if kiosk != nil {
		locationID = kiosk.LocationID
		kioskActor = "kiosk:" + kiosk.Code
	}
	if locationID == "" {
		if loc, _ := s.locations.GetDefault(); loc != nil {
			locationID = loc.ID
		}
	}

	now := time.Now()
	visit := &entity.LibraryVisit{
		LocationID: locationID,
		CheckInAt:  now,
		Status:     "recorded",
		CreatedBy:  kioskActor,
	}
	if kiosk != nil {
		visit.KioskID = kiosk.ID
	}
	if purpose != nil {
		visit.PurposeID = purpose.ID
	}
	if in.ClientEventID != "" {
		visit.ClientEventID = in.ClientEventID
	}

	switch in.Method {
	case "nfc":
		if s.hmacKey == "" {
			return nil, ErrHMACKeyMissing
		}
		lookup := credhash.Lookup(s.hmacKey, credhash.Normalize(in.Credential))
		card, err := s.cards.GetByLookup(lookup)
		if err != nil {
			return nil, err
		}
		if card == nil {
			return nil, ErrUnknownCredential
		}
		if card.Status != "active" {
			return nil, ErrCardNotActive
		}
		member, err := s.members.GetByID(card.MemberID)
		if err != nil {
			return nil, err
		}
		if member == nil {
			return nil, ErrUnknownCredential
		}
		if member.Status != "active" {
			return nil, ErrMemberInactive
		}
		visit.EntryMethod = "nfc"
		visit.VisitorType = member.MemberType
		visit.MemberID = member.ID
		visit.SnapshotName = member.DisplayName
		visit.SnapshotFaculty = member.FacultyName
		visit.SnapshotStudyProgram = member.StudyProgramName

	case "member_number":
		nim := strings.TrimSpace(in.Credential)
		key := kioskActor + "|" + nim
		if !s.limiter.Allow(key) {
			return nil, ErrRateLimited
		}
		member, err := s.members.GetByNumber(nim)
		if err != nil {
			return nil, err
		}
		if member == nil {
			return nil, ErrUnknownCredential
		}
		if member.Status != "active" {
			return nil, ErrMemberInactive
		}
		visit.EntryMethod = "member_number"
		visit.FallbackReason = "forgot_card"
		visit.VisitorType = member.MemberType
		visit.MemberID = member.ID
		visit.SnapshotName = member.DisplayName
		visit.SnapshotFaculty = member.FacultyName
		visit.SnapshotStudyProgram = member.StudyProgramName

	case "guest_barcode":
		if s.hmacKey == "" {
			return nil, ErrHMACKeyMissing
		}
		token := strings.TrimSpace(in.Credential)
		lookup := credhash.Lookup(s.hmacKey, token)
		pass, err := s.passes.GetByLookup(lookup)
		if err != nil {
			return nil, err
		}
		if pass == nil || pass.Status != "active" || !now.Before(pass.ExpiresAt) {
			return nil, ErrPassInvalid
		}
		if pass.MaxUses > 0 && pass.UseCount >= pass.MaxUses {
			return nil, ErrPassInvalid
		}
		pass.UseCount++
		if pass.MaxUses > 0 && pass.UseCount >= pass.MaxUses {
			pass.Status = "used"
		}
		if err := s.passes.Update(pass); err != nil {
			return nil, err
		}
		guest, err := s.guests.GetByID(pass.GuestID)
		if err != nil {
			return nil, err
		}
		if guest == nil {
			return nil, ErrPassInvalid
		}
		visit.EntryMethod = "guest_barcode"
		visit.VisitorType = "guest"
		visit.GuestID = guest.ID
		visit.SnapshotName = guest.DisplayName
		visit.SnapshotUnit = guest.Organization
	}

	// 4. Dedup: kunjungan sama orang+lokasi dalam jendela → tidak buat row baru.
	if visit.MemberID != "" {
		if recent, _ := s.visits.RecentByMemberLocation(visit.MemberID, locationID, DedupWindow); recent != nil {
			return s.resultFromVisit(recent, true), nil
		}
	} else if visit.GuestID != "" {
		if recent, _ := s.visits.RecentByGuestLocation(visit.GuestID, locationID, DedupWindow); recent != nil {
			return s.resultFromVisit(recent, true), nil
		}
	}

	// 5. Simpan.
	if err := s.visits.Create(visit); err != nil {
		return nil, err
	}
	return s.resultFromVisit(visit, false), nil
}

func (s *CheckinService) resultFromVisit(v *entity.LibraryVisit, duplicate bool) *CheckinResult {
	res := &CheckinResult{
		VisitID:      v.ID,
		VisitorName:  v.SnapshotName,
		VisitorType:  v.VisitorType,
		StudyProgram: v.SnapshotStudyProgram,
		Faculty:      v.SnapshotFaculty,
		Duplicate:    duplicate,
	}
	if v.PurposeID != "" {
		if ps, _ := s.purposes.ListActive(); ps != nil {
			for _, p := range ps {
				if p.ID == v.PurposeID {
					res.Purpose = p.Name
					break
				}
			}
		}
	}
	return res
}

// ===== Rate limiter NIM (in-memory, per kiosk+NIM, 5x/menit) =====

type nimLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newNimLimiter() *nimLimiter {
	return &nimLimiter{hits: make(map[string][]time.Time)}
}

func (l *nimLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	fresh := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < time.Minute {
			fresh = append(fresh, t)
		}
	}
	if len(fresh) >= 5 {
		l.hits[key] = fresh
		return false
	}
	fresh = append(fresh, now)
	l.hits[key] = fresh
	return true
}
