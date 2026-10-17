package repository

import (
	"time"

	"github.com/rama/b-wise/library/internal/domain/entity"
)

// ===== Anggota =====

type MemberFilter struct {
	Search string // NIM atau nama (ILIKE)
	Status string
	Page   int
	Limit  int
}

type MemberRepository interface {
	Create(m *entity.LibraryMember) error
	Update(m *entity.LibraryMember) error
	GetByID(id string) (*entity.LibraryMember, error)
	GetByNumber(number string) (*entity.LibraryMember, error)
	List(filter MemberFilter) ([]entity.LibraryMember, int64, error)
}

type MemberCardRepository interface {
	Create(c *entity.LibraryMemberCard) error
	Update(c *entity.LibraryMemberCard) error
	GetByID(id string) (*entity.LibraryMemberCard, error)
	GetByLookup(lookup string) (*entity.LibraryMemberCard, error)
	ActiveByMember(memberID string) (*entity.LibraryMemberCard, error)
	ListByMember(memberID string) ([]entity.LibraryMemberCard, error)
}

// ===== Tamu & pass =====

type GuestRepository interface {
	Create(g *entity.LibraryGuest) error
	GetByID(id string) (*entity.LibraryGuest, error)
	List(search string, page, limit int) ([]entity.LibraryGuest, int64, error)
}

type GuestPassRepository interface {
	Create(p *entity.LibraryGuestPass) error
	Update(p *entity.LibraryGuestPass) error
	GetByID(id string) (*entity.LibraryGuestPass, error)
	GetByLookup(lookup string) (*entity.LibraryGuestPass, error)
	ListByGuest(guestID string) ([]entity.LibraryGuestPass, error)
}

// ===== Master =====

type VisitPurposeRepository interface {
	ListActive() ([]entity.LibraryVisitPurpose, error)
	GetByCode(code string) (*entity.LibraryVisitPurpose, error)
}

type LocationRepository interface {
	GetDefault() (*entity.LibraryLocation, error)
	List() ([]entity.LibraryLocation, error)
}

type KioskFilter struct {
	LocationID string
}

type KioskRepository interface {
	Create(k *entity.LibraryKiosk) error
	Update(k *entity.LibraryKiosk) error
	GetByCode(code string) (*entity.LibraryKiosk, error)
	GetByID(id string) (*entity.LibraryKiosk, error)
	List() ([]entity.LibraryKiosk, error)
	Touch(id, appVersion string) error
	Delete(id string) error
}

// ===== Kunjungan =====

type VisitFilter struct {
	LocationID  string
	VisitorType string
	Status      string
	Search      string
	MemberID    string
	From        string // RFC3339
	To          string // RFC3339
	Page        int
	Limit       int
}

type VisitRepository interface {
	Create(v *entity.LibraryVisit) error
	Update(v *entity.LibraryVisit) error
	GetByID(id string) (*entity.LibraryVisit, error)
	GetByClientEventID(id string) (*entity.LibraryVisit, error)
	RecentByMemberLocation(memberID, locationID string, within time.Duration) (*entity.LibraryVisit, error)
	RecentByGuestLocation(guestID, locationID string, within time.Duration) (*entity.LibraryVisit, error)
	CountByKiosk(kioskID string) (int64, error)
	List(filter VisitFilter) ([]entity.LibraryVisit, int64, error)
}

// ===== Audit =====

type AuditLogRepository interface {
	Create(a *entity.LibraryAuditLog) error
}
