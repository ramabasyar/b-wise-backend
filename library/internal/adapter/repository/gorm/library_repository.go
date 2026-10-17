package gorm

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/rama/b-wise/library/internal/domain/entity"
	"github.com/rama/b-wise/library/internal/domain/repository"
)

// Konvensi: method Get* mengembalikan (nil, nil) saat record tidak ditemukan.

func pageLimit(page, limit, def, max int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = def
	}
	if limit > max {
		limit = max
	}
	return page, limit
}

// ===== Member =====

type MemberRepository struct{ db *gorm.DB }

func NewMemberRepository(db *gorm.DB) repository.MemberRepository {
	return &MemberRepository{db: db}
}

func (r *MemberRepository) Create(m *entity.LibraryMember) error { return r.db.Create(m).Error }

func (r *MemberRepository) Update(m *entity.LibraryMember) error { return r.db.Save(m).Error }

func (r *MemberRepository) GetByID(id string) (*entity.LibraryMember, error) {
	var m entity.LibraryMember
	if err := r.db.First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *MemberRepository) GetByNumber(number string) (*entity.LibraryMember, error) {
	var m entity.LibraryMember
	if err := r.db.First(&m, "member_number = ?", number).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (r *MemberRepository) List(f repository.MemberFilter) ([]entity.LibraryMember, int64, error) {
	page, limit := pageLimit(f.Page, f.Limit, 20, 100)
	q := r.db.Model(&entity.LibraryMember{})
	if s := strings.TrimSpace(f.Search); s != "" {
		pat := "%" + s + "%"
		q = q.Where("member_number ILIKE ? OR display_name ILIKE ?", pat, pat)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []entity.LibraryMember
	if err := q.Omit("photo").Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ===== MemberCard =====

type MemberCardRepository struct{ db *gorm.DB }

func NewMemberCardRepository(db *gorm.DB) repository.MemberCardRepository {
	return &MemberCardRepository{db: db}
}

func (r *MemberCardRepository) Create(c *entity.LibraryMemberCard) error { return r.db.Create(c).Error }

func (r *MemberCardRepository) Update(c *entity.LibraryMemberCard) error { return r.db.Save(c).Error }

func (r *MemberCardRepository) GetByID(id string) (*entity.LibraryMemberCard, error) {
	var c entity.LibraryMemberCard
	if err := r.db.First(&c, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *MemberCardRepository) GetByLookup(lookup string) (*entity.LibraryMemberCard, error) {
	var c entity.LibraryMemberCard
	if err := r.db.First(&c, "credential_lookup = ?", lookup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *MemberCardRepository) ActiveByMember(memberID string) (*entity.LibraryMemberCard, error) {
	var c entity.LibraryMemberCard
	if err := r.db.First(&c, "member_id = ? AND status = ?", memberID, "active").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *MemberCardRepository) ListByMember(memberID string) ([]entity.LibraryMemberCard, error) {
	var rows []entity.LibraryMemberCard
	if err := r.db.Where("member_id = ?", memberID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ===== Guest =====

type GuestRepository struct{ db *gorm.DB }

func NewGuestRepository(db *gorm.DB) repository.GuestRepository {
	return &GuestRepository{db: db}
}

func (r *GuestRepository) Create(g *entity.LibraryGuest) error { return r.db.Create(g).Error }

func (r *GuestRepository) GetByID(id string) (*entity.LibraryGuest, error) {
	var g entity.LibraryGuest
	if err := r.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &g, nil
}

func (r *GuestRepository) List(search string, page, limit int) ([]entity.LibraryGuest, int64, error) {
	page, limit = pageLimit(page, limit, 20, 100)
	q := r.db.Model(&entity.LibraryGuest{})
	if s := strings.TrimSpace(search); s != "" {
		pat := "%" + s + "%"
		q = q.Where("display_name ILIKE ? OR organization ILIKE ?", pat, pat)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []entity.LibraryGuest
	if err := q.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ===== GuestPass =====

type GuestPassRepository struct{ db *gorm.DB }

func NewGuestPassRepository(db *gorm.DB) repository.GuestPassRepository {
	return &GuestPassRepository{db: db}
}

func (r *GuestPassRepository) Create(p *entity.LibraryGuestPass) error { return r.db.Create(p).Error }

func (r *GuestPassRepository) Update(p *entity.LibraryGuestPass) error { return r.db.Save(p).Error }

func (r *GuestPassRepository) GetByID(id string) (*entity.LibraryGuestPass, error) {
	var p entity.LibraryGuestPass
	if err := r.db.First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *GuestPassRepository) GetByLookup(lookup string) (*entity.LibraryGuestPass, error) {
	var p entity.LibraryGuestPass
	if err := r.db.First(&p, "token_lookup = ?", lookup).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *GuestPassRepository) ListByGuest(guestID string) ([]entity.LibraryGuestPass, error) {
	var rows []entity.LibraryGuestPass
	if err := r.db.Where("guest_id = ?", guestID).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ===== VisitPurpose =====

type VisitPurposeRepository struct{ db *gorm.DB }

func NewVisitPurposeRepository(db *gorm.DB) repository.VisitPurposeRepository {
	return &VisitPurposeRepository{db: db}
}

func (r *VisitPurposeRepository) ListActive() ([]entity.LibraryVisitPurpose, error) {
	var rows []entity.LibraryVisitPurpose
	if err := r.db.Where("is_active = ?", true).Order("sort_order ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *VisitPurposeRepository) GetByCode(code string) (*entity.LibraryVisitPurpose, error) {
	var p entity.LibraryVisitPurpose
	if err := r.db.First(&p, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ===== Location =====

type LocationRepository struct{ db *gorm.DB }

func NewLocationRepository(db *gorm.DB) repository.LocationRepository {
	return &LocationRepository{db: db}
}

func (r *LocationRepository) GetDefault() (*entity.LibraryLocation, error) {
	var l entity.LibraryLocation
	if err := r.db.First(&l, "is_active = ?", true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &l, nil
}

func (r *LocationRepository) List() ([]entity.LibraryLocation, error) {
	var rows []entity.LibraryLocation
	if err := r.db.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ===== Kiosk =====

type KioskRepository struct{ db *gorm.DB }

func NewKioskRepository(db *gorm.DB) repository.KioskRepository {
	return &KioskRepository{db: db}
}

func (r *KioskRepository) Create(k *entity.LibraryKiosk) error { return r.db.Create(k).Error }

func (r *KioskRepository) GetByCode(code string) (*entity.LibraryKiosk, error) {
	var k entity.LibraryKiosk
	if err := r.db.First(&k, "code = ?", code).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &k, nil
}

func (r *KioskRepository) GetByID(id string) (*entity.LibraryKiosk, error) {
	var k entity.LibraryKiosk
	if err := r.db.First(&k, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &k, nil
}

func (r *KioskRepository) List() ([]entity.LibraryKiosk, error) {
	var rows []entity.LibraryKiosk
	if err := r.db.Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *KioskRepository) Update(k *entity.LibraryKiosk) error { return r.db.Save(k).Error }

func (r *KioskRepository) Delete(id string) error {
	return r.db.Delete(&entity.LibraryKiosk{}, "id = ?", id).Error
}

func (r *KioskRepository) Touch(id, appVersion string) error {
	updates := map[string]interface{}{"last_seen_at": time.Now()}
	if appVersion != "" {
		updates["app_version"] = appVersion
	}
	return r.db.Model(&entity.LibraryKiosk{}).Where("id = ?", id).Updates(updates).Error
}

// ===== Visit =====

type VisitRepository struct{ db *gorm.DB }

func NewVisitRepository(db *gorm.DB) repository.VisitRepository {
	return &VisitRepository{db: db}
}

func (r *VisitRepository) Create(v *entity.LibraryVisit) error { return r.db.Create(v).Error }

func (r *VisitRepository) Update(v *entity.LibraryVisit) error { return r.db.Save(v).Error }

func (r *VisitRepository) GetByID(id string) (*entity.LibraryVisit, error) {
	var v entity.LibraryVisit
	if err := r.db.First(&v, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *VisitRepository) GetByClientEventID(id string) (*entity.LibraryVisit, error) {
	if id == "" {
		return nil, nil
	}
	var v entity.LibraryVisit
	if err := r.db.First(&v, "client_event_id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *VisitRepository) CountByKiosk(kioskID string) (int64, error) {
	var n int64
	if err := r.db.Model(&entity.LibraryVisit{}).Where("kiosk_id = ?", kioskID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *VisitRepository) recent(where string, args []interface{}, within time.Duration) (*entity.LibraryVisit, error) {
	var v entity.LibraryVisit
	args = append(args, "recorded", time.Now().Add(-within))
	q := r.db.Where(where+" AND status = ? AND check_in_at > ?", args...).Order("check_in_at DESC").First(&v)
	if q.Error != nil {
		if errors.Is(q.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, q.Error
	}
	return &v, nil
}

func (r *VisitRepository) RecentByMemberLocation(memberID, locationID string, within time.Duration) (*entity.LibraryVisit, error) {
	return r.recent("member_id = ?", []interface{}{memberID}, within)
}

func (r *VisitRepository) RecentByGuestLocation(guestID, locationID string, within time.Duration) (*entity.LibraryVisit, error) {
	return r.recent("guest_id = ?", []interface{}{guestID}, within)
}

func (r *VisitRepository) List(f repository.VisitFilter) ([]entity.LibraryVisit, int64, error) {
	page, limit := pageLimit(f.Page, f.Limit, 20, 100)
	q := r.db.Model(&entity.LibraryVisit{})
	if f.LocationID != "" {
		q = q.Where("location_id = ?", f.LocationID)
	}
	if f.VisitorType != "" {
		q = q.Where("visitor_type = ?", f.VisitorType)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.MemberID != "" {
		q = q.Where("member_id = ?", f.MemberID)
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		pat := "%" + s + "%"
		q = q.Where("snapshot_name ILIKE ?", pat)
	}
	if f.From != "" {
		if t, err := time.Parse(time.RFC3339, f.From); err == nil {
			q = q.Where("check_in_at >= ?", t)
		}
	}
	if f.To != "" {
		if t, err := time.Parse(time.RFC3339, f.To); err == nil {
			q = q.Where("check_in_at <= ?", t)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []entity.LibraryVisit
	if err := q.Order("check_in_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// ===== AuditLog =====

type AuditLogRepository struct{ db *gorm.DB }

func NewAuditLogRepository(db *gorm.DB) repository.AuditLogRepository {
	return &AuditLogRepository{db: db}
}

func (r *AuditLogRepository) Create(a *entity.LibraryAuditLog) error { return r.db.Create(a).Error }
