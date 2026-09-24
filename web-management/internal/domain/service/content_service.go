package service

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm/clause"
	"gorm.io/gorm"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

// ContentService — logika konten BWM: CRUD, lifecycle (draft→published→archived),
// kueri publik (hanya konten live), slug unik, dan invalidasi cache publik.
type ContentService struct {
	db        *gorm.DB
	redis     *redis.Client
	tax       *TaxonomyService
	webhooks  []string
}

func NewContentService(db *gorm.DB, r *redis.Client) *ContentService {
	return &ContentService{db: db, redis: r, tax: NewTaxonomyService(db)}
}

// Tax — akses layanan taksonomi (kategori & tag).
func (s *ContentService) Tax() *TaxonomyService { return s.tax }

// WithWebhooks — daftar URL yang dikabari saat publish/archive (chainable).
func (s *ContentService) WithWebhooks(urls []string) *ContentService {
	s.webhooks = urls
	return s
}

var (
	ErrNotFound = errors.New("konten tidak ditemukan")
	ErrInvalid  = errors.New("input tidak valid")
)

// FlushPublicCache — panggil setelah semua mutasi konten (publish/edit/hapus).
func (s *ContentService) FlushPublicCache() {
	if s.redis == nil {
		return
	}
	ctx := context.Background()
	keys, err := s.redis.Keys(ctx, "bwm:pub:*").Result()
	if err == nil && len(keys) > 0 {
		_ = s.redis.Del(ctx, keys...).Err()
	}
}

// ==================== Input ====================

type PostTrInput struct {
	Title    string         `json:"title"`
	Excerpt  string         `json:"excerpt"`
	Author   string         `json:"author"`
	Category string         `json:"category"`
	HeroAlt  string         `json:"hero_alt"`
	Blocks   []blocks.Block `json:"blocks"`
}

type PostInput struct {
	Slug         string                  `json:"slug"`
	Type         string                  `json:"type"`
	Featured     bool                    `json:"featured"`
	HeroURL      string                  `json:"hero_url"`
	HeroMediaID  *string                 `json:"hero_media_id"`
	PublishAt    *time.Time              `json:"publish_at"`
	UnpublishAt  *time.Time              `json:"unpublish_at"`
	Translations map[string]PostTrInput  `json:"translations"`
	SEO          entity.SEO              `json:"seo"`
}

type PageTrInput struct {
	Title  string         `json:"title"`
	Blocks []blocks.Block `json:"blocks"`
}

type PageInput struct {
	Slug         string                 `json:"slug"`
	ParentID     *string                `json:"parent_id"`
	Template     string                 `json:"template"`
	PublishAt    *time.Time             `json:"publish_at"`
	UnpublishAt  *time.Time             `json:"unpublish_at"`
	Translations map[string]PageTrInput `json:"translations"`
	SEO          entity.SEO             `json:"seo"`
}

type EventTrInput struct {
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle"`
	Description string `json:"description"`
}

type EventInput struct {
	Slug         string                  `json:"slug"`
	EventDate    *time.Time              `json:"event_date"`
	Location     string                  `json:"location"`
	HeroURL      string                  `json:"hero_url"`
	HeroMediaID  *string                 `json:"hero_media_id"`
	PublishAt    *time.Time              `json:"publish_at"`
	UnpublishAt  *time.Time              `json:"unpublish_at"`
	Translations map[string]EventTrInput `json:"translations"`
	SEO          entity.SEO              `json:"seo"`
}

type BannerTrInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type BannerInput struct {
	ImageURL     string                   `json:"image_url"`
	ImageMediaID *string                  `json:"image_media_id"`
	Link         string                   `json:"link"`
	SortOrder    int                      `json:"sort_order"`
	Active       *bool                    `json:"active"`
	StartAt      *time.Time               `json:"start_at"`
	EndAt        *time.Time               `json:"end_at"`
	Translations map[string]BannerTrInput `json:"translations"`
}

type DocumentInput struct {
	Name         string `json:"name"`
	AcademicYear string `json:"academic_year"`
	GroupLabel   string `json:"group_label"`
	SortOrder    int    `json:"sort_order"`
	FileURL      string `json:"file_url"`
	FileType     string `json:"file_type"`
	Active       *bool  `json:"active"`
}

// ==================== slug ====================

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = regexp.MustCompile(`[\s_]+`).ReplaceAllString(s, "-")
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 180 {
		s = s[:180]
	}
	return s
}

func (s *ContentService) uniqueSlug(base, table string) string {
	slug := Slugify(base)
	if slug == "" {
		slug = fmt.Sprintf("konten-%d", time.Now().UnixMilli())
	}
	for i := 0; i < 200; i++ {
		var n int64
		q := s.db.Table(table).Where("slug = ?", slug).Count(&n)
		if q.Error == nil && n == 0 {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", Slugify(base), i+2)
	}
	return fmt.Sprintf("%s-%d", slug, time.Now().UnixMilli())
}

// ==================== Post ====================

func (s *ContentService) ListPosts(status, typ, q string, page, per int) ([]entity.Post, int64, error) {
	tx := s.db.Model(&entity.Post{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if typ != "" {
		tx = tx.Where("type = ?", typ)
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("lower(translations::text) LIKE ?", like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 20
	}
	var rows []entity.Post
	err := tx.Order("updated_at DESC").Limit(per).Offset((page - 1) * per).Find(&rows).Error
	if err == nil {
		s.tax.PopulateTerms(rows)
	}
	return rows, total, err
}

func (s *ContentService) GetPost(id string) (*entity.Post, error) {
	var p entity.Post
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	s.tax.PopulateTermsOne(&p)
	return &p, nil
}

func validatePostTr(in map[string]PostTrInput) error {
	id, ok := in["id"]
	if !ok || strings.TrimSpace(id.Title) == "" {
		return fmt.Errorf("title locale %q wajib ada: %w", "id", ErrInvalid)
	}
	for loc, t := range in {
		if err := blocks.Validate(t.Blocks); err != nil {
			return fmt.Errorf("blocks locale %q: %w", loc, err)
		}
	}
	return nil
}

func (s *ContentService) CreatePost(in PostInput, actor string) (*entity.Post, error) {
	if err := validatePostTr(in.Translations); err != nil {
		return nil, err
	}
	if in.Type == "" {
		in.Type = "news"
	}
	p := &entity.Post{
		Type: in.Type, Status: entity.StatusDraft, Featured: in.Featured,
		HeroURL: in.HeroURL, HeroMediaID: in.HeroMediaID,
		PublishAt: in.PublishAt, UnpublishAt: in.UnpublishAt,
		SEO: marshalSEO(in.SEO), Version: 1, CreatedBy: actor, UpdatedBy: actor,
		Source: "manual",
	}
	for loc, t := range in.Translations {
		p.SetTr(loc, entity.PostTranslation(t))
	}
	if in.Slug != "" {
		p.Slug = Slugify(in.Slug)
	} else {
		p.Slug = s.uniqueSlug(in.Translations["id"].Title, "posts")
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		return s.saveVersion(tx, "post", p.ID, p.Version, postSnapshotOf(p), actor)
	}); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *ContentService) UpdatePost(id string, in PostInput, actor string) (*entity.Post, error) {
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	if err := validatePostTr(in.Translations); err != nil {
		return nil, err
	}
	oldSlug := p.Slug
	if in.Slug != "" && Slugify(in.Slug) != p.Slug {
		p.Slug = s.uniqueSlug(in.Slug, "posts")
	}
	if in.Type != "" {
		p.Type = in.Type
	}
	p.Featured = in.Featured
	p.HeroURL = in.HeroURL
	p.HeroMediaID = in.HeroMediaID
	p.PublishAt = in.PublishAt
	p.UnpublishAt = in.UnpublishAt
	p.SEO = marshalSEO(in.SEO)
	p.Version++
	p.UpdatedBy = actor
	p.Translations = "{}"
	for loc, t := range in.Translations {
		p.SetTr(loc, entity.PostTranslation(t))
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(p).Error; err != nil {
			return err
		}
		return s.saveVersion(tx, "post", p.ID, p.Version, postSnapshotOf(p), actor)
	}); err != nil {
		return nil, err
	}
	if p.Slug != oldSlug {
		s.ensurePostRedirect(oldSlug, p.Slug, actor)
	}
	s.FlushPublicCache()
	return p, nil
}

func (s *ContentService) DeletePost(id string) error {
	p, err := s.GetPost(id)
	if err != nil {
		return err
	}
	if err := s.db.Delete(p).Error; err != nil {
		return err
	}
	s.FlushPublicCache()
	return nil
}

func (s *ContentService) PublishPost(id, actor string, at *time.Time) (*entity.Post, error) {
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	if p.Status == entity.StatusArchived {
		return nil, fmt.Errorf("konten terarsip — duplikat utk menerbitkan ulang: %w", ErrInvalid)
	}
	now := time.Now()
	if at == nil {
		at = &now
	}
	if at.After(now) {
		p.Status = entity.StatusScheduled // F3: terbit otomatis via scheduler saat jatuh tempo
	} else {
		p.Status = entity.StatusPublished
	}
	p.PublishAt = at
	p.PublishedVersion = p.Version
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
	}
	if p.Status != entity.StatusPublished {
		return p, nil // scheduled: tanpa flush/webhook — saat tayang nanti
	}
	s.FlushPublicCache()
	FireWebhooks(s.webhooks, PublishEvent{Event: "publish", Entity: "post", ID: p.ID, Slug: p.Slug, At: time.Now()})
	return p, nil
}

func (s *ContentService) ArchivePost(id, actor string) (*entity.Post, error) {
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	p.Status = entity.StatusArchived
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	FireWebhooks(s.webhooks, PublishEvent{Event: "archive", Entity: "post", ID: p.ID, Slug: p.Slug, At: time.Now()})
	return p, nil
}

// ==================== Event ====================

func (s *ContentService) ListEvents(status, q string, page, per int) ([]entity.Event, int64, error) {
	tx := s.db.Model(&entity.Event{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 20
	}
	var rows []entity.Event
	err := tx.Order("event_date DESC").Limit(per).Offset((page - 1) * per).Find(&rows).Error
	return rows, total, err
}

func (s *ContentService) GetEvent(id string) (*entity.Event, error) {
	var e entity.Event
	if err := s.db.First(&e, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &e, nil
}

func (s *ContentService) CreateEvent(in EventInput, actor string) (*entity.Event, error) {
	if tr, ok := in.Translations["id"]; !ok || strings.TrimSpace(tr.Title) == "" {
		return nil, fmt.Errorf("title locale id wajib: %w", ErrInvalid)
	}
	e := &entity.Event{
		Status: entity.StatusDraft, EventDate: in.EventDate, Location: in.Location,
		HeroURL: in.HeroURL, HeroMediaID: in.HeroMediaID,
		PublishAt: in.PublishAt, UnpublishAt: in.UnpublishAt,
		SEO: marshalSEO(in.SEO), Version: 1, CreatedBy: actor, UpdatedBy: actor,
	}
	if e.EventDate != nil {
		e.Month = indonesianMonth(e.EventDate.Month())
		e.Year = e.EventDate.Year()
	}
	for loc, t := range in.Translations {
		e.SetTr(loc, entity.EventTranslation(t))
	}
	if in.Slug != "" {
		e.Slug = Slugify(in.Slug)
	} else {
		e.Slug = s.uniqueSlug(in.Translations["id"].Title, "events")
	}
	if err := s.db.Create(e).Error; err != nil {
		return nil, err
	}
	return e, nil
}

func (s *ContentService) UpdateEvent(id string, in EventInput, actor string) (*entity.Event, error) {
	e, err := s.GetEvent(id)
	if err != nil {
		return nil, err
	}
	if in.Slug != "" && Slugify(in.Slug) != e.Slug {
		e.Slug = s.uniqueSlug(in.Slug, "events")
	}
	e.EventDate = in.EventDate
	if e.EventDate != nil {
		e.Month = indonesianMonth(e.EventDate.Month())
		e.Year = e.EventDate.Year()
	}
	e.Location = in.Location
	e.HeroURL = in.HeroURL
	e.HeroMediaID = in.HeroMediaID
	e.PublishAt = in.PublishAt
	e.UnpublishAt = in.UnpublishAt
	e.SEO = marshalSEO(in.SEO)
	e.Version++
	e.UpdatedBy = actor
	e.Translations = "{}"
	for loc, t := range in.Translations {
		e.SetTr(loc, entity.EventTranslation(t))
	}
	if err := s.db.Save(e).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	return e, nil
}

func (s *ContentService) DeleteEvent(id string) error {
	e, err := s.GetEvent(id)
	if err != nil {
		return err
	}
	if err := s.db.Delete(e).Error; err != nil {
		return err
	}
	s.FlushPublicCache()
	return nil
}

func (s *ContentService) PublishEvent(id, actor string, at *time.Time) (*entity.Event, error) {
	e, err := s.GetEvent(id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if at == nil {
		at = &now
	}
	e.Status = entity.StatusPublished
	e.PublishAt = at
	e.PublishedVersion = e.Version
	e.UpdatedBy = actor
	if err := s.db.Save(e).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	FireWebhooks(s.webhooks, PublishEvent{Event: "publish", Entity: "event", ID: e.ID, Slug: e.Slug, At: time.Now()})
	return e, nil
}

func (s *ContentService) ArchiveEvent(id, actor string) (*entity.Event, error) {
	e, err := s.GetEvent(id)
	if err != nil {
		return nil, err
	}
	e.Status = entity.StatusArchived
	e.UpdatedBy = actor
	if err := s.db.Save(e).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	return e, nil
}

// ==================== Banner ====================

func (s *ContentService) ListBanners() ([]entity.Banner, error) {
	var rows []entity.Banner
	err := s.db.Order("sort_order ASC, created_at ASC").Find(&rows).Error
	return rows, err
}

func (s *ContentService) CreateBanner(in BannerInput, actor string) (*entity.Banner, error) {
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	b := &entity.Banner{
		ImageURL: in.ImageURL, ImageMediaID: in.ImageMediaID, Link: in.Link,
		SortOrder: in.SortOrder, Active: active,
		StartAt: in.StartAt, EndAt: in.EndAt, CreatedBy: actor, UpdatedBy: actor,
	}
	for loc, t := range in.Translations {
		b.SetTr(loc, entity.BannerTranslation(t))
	}
	if err := s.db.Create(b).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	return b, nil
}

func (s *ContentService) UpdateBanner(id string, in BannerInput, actor string) (*entity.Banner, error) {
	var b entity.Banner
	if err := s.db.First(&b, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	b.ImageURL = in.ImageURL
	b.ImageMediaID = in.ImageMediaID
	b.Link = in.Link
	b.SortOrder = in.SortOrder
	if in.Active != nil {
		b.Active = *in.Active
	}
	b.StartAt = in.StartAt
	b.EndAt = in.EndAt
	b.UpdatedBy = actor
	b.Translations = "{}"
	for loc, t := range in.Translations {
		b.SetTr(loc, entity.BannerTranslation(t))
	}
	if err := s.db.Save(&b).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	return &b, nil
}

func (s *ContentService) DeleteBanner(id string) error {
	res := s.db.Delete(&entity.Banner{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	s.FlushPublicCache()
	return nil
}

// ==================== Page ====================

func (s *ContentService) ListPages(status string, page, per int) ([]entity.Page, int64, error) {
	tx := s.db.Model(&entity.Page{})
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 100 {
		per = 20
	}
	var rows []entity.Page
	err := tx.Order("updated_at DESC").Limit(per).Offset((page-1)*per).Find(&rows).Error
	return rows, total, err
}

func (s *ContentService) GetPage(id string) (*entity.Page, error) {
	var p entity.Page
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *ContentService) UpsertPage(in PageInput, actor string, id string) (*entity.Page, error) {
	if tr, ok := in.Translations["id"]; !ok || strings.TrimSpace(tr.Title) == "" {
		return nil, fmt.Errorf("title locale id wajib: %w", ErrInvalid)
	}
	for loc, t := range in.Translations {
		if err := blocks.Validate(t.Blocks); err != nil {
			return nil, fmt.Errorf("blocks locale %q: %w", loc, err)
		}
	}
	var p entity.Page
	creating := id == ""
	if !creating {
		got, err := s.GetPage(id)
		if err != nil {
			return nil, err
		}
		p = *got
	}
	if in.Template != "" {
		p.Template = in.Template
	}
	p.ParentID = in.ParentID
	p.PublishAt = in.PublishAt
	p.UnpublishAt = in.UnpublishAt
	p.SEO = marshalSEO(in.SEO)
	p.UpdatedBy = actor
	p.Translations = "{}"
	for loc, t := range in.Translations {
		p.SetTr(loc, entity.PageTranslation(t))
	}
	if creating {
		p.Status = entity.StatusDraft
		p.Version = 1
		p.CreatedBy = actor
		if in.Slug != "" {
			p.Slug = Slugify(in.Slug)
		} else {
			p.Slug = s.uniqueSlug(in.Translations["id"].Title, "pages")
		}
		if err := s.db.Create(&p).Error; err != nil {
			return nil, err
		}
	} else {
		if in.Slug != "" && Slugify(in.Slug) != p.Slug {
			p.Slug = s.uniqueSlug(in.Slug, "pages")
		}
		p.Version++
		if err := s.db.Save(&p).Error; err != nil {
			return nil, err
		}
	}
	s.FlushPublicCache()
	return &p, nil
}

func (s *ContentService) DeletePage(id string) error {
	if err := s.db.Delete(&entity.Page{}, "id = ?", id).Error; err != nil {
		return err
	}
	s.FlushPublicCache()
	return nil
}

func (s *ContentService) PublishPage(id, actor string, at *time.Time) (*entity.Page, error) {
	p, err := s.GetPage(id)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if at == nil {
		at = &now
	}
	p.Status = entity.StatusPublished
	p.PublishAt = at
	p.PublishedVersion = p.Version
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	FireWebhooks(s.webhooks, PublishEvent{Event: "publish", Entity: "page", ID: p.ID, Slug: p.Slug, At: time.Now()})
	return p, nil
}

// ==================== Document ====================

func (s *ContentService) ListDocuments() ([]entity.Document, error) {
	var rows []entity.Document
	err := s.db.Order("academic_year DESC, group_label ASC, sort_order ASC").Find(&rows).Error
	return rows, err
}

func (s *ContentService) UpsertDocument(in DocumentInput, actor string, id string) (*entity.Document, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name wajib: %w", ErrInvalid)
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	d := entity.Document{
		Name: in.Name, AcademicYear: in.AcademicYear, GroupLabel: in.GroupLabel,
		SortOrder: in.SortOrder, FileURL: in.FileURL, FileType: in.FileType, Active: active,
	}
	if id != "" {
		d.ID = id
		if err := s.db.First(&entity.Document{}, "id = ?", id).Error; err != nil {
			return nil, ErrNotFound
		}
		d.UpdatedAt = time.Now()
		if err := s.db.Model(&entity.Document{}).Where("id = ?", id).Updates(map[string]any{
			"name": d.Name, "academic_year": d.AcademicYear, "group_label": d.GroupLabel,
			"sort_order": d.SortOrder, "file_url": d.FileURL, "file_type": d.FileType, "active": d.Active,
		}).Error; err != nil {
			return nil, err
		}
	} else {
		if err := s.db.Create(&d).Error; err != nil {
			return nil, err
		}
	}
	s.FlushPublicCache()
	return &d, nil
}

func (s *ContentService) DeleteDocument(id string) error {
	if err := s.db.Delete(&entity.Document{}, "id = ?", id).Error; err != nil {
		return err
	}
	s.FlushPublicCache()
	return nil
}

// ==================== kueri PUBLIK (live only) ====================

type PublicPost struct {
	Slug      string         `json:"slug"`
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Excerpt   string         `json:"excerpt,omitempty"`
	Author    string         `json:"author,omitempty"`
	Category  string         `json:"category,omitempty"`
	HeroURL   string         `json:"hero_url,omitempty"`
	HeroAlt   string         `json:"hero_alt,omitempty"`
	Blocks    []blocks.Block `json:"blocks,omitempty"`
	Locale    string         `json:"locale"`
	Featured  bool           `json:"featured"`
	SEO       entity.SEO     `json:"seo,omitempty"`
	PublishedAt *time.Time   `json:"published_at,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func (s *ContentService) publicPostOf(p *entity.Post, locale string, withBlocks bool) PublicPost {
	tr, resolved := entity.TrResolve(entity.TrOf[entity.PostTranslation](p.Translations), locale)
	out := PublicPost{
		Slug: p.Slug, Type: p.Type, Title: tr.Title, Excerpt: tr.Excerpt,
		Author: tr.Author, Category: tr.Category, HeroURL: p.HeroURL, HeroAlt: tr.HeroAlt,
		Locale: resolved, Featured: p.Featured, SEO: entity.SEOOf(p.SEO),
		PublishedAt: p.PublishAt, UpdatedAt: p.UpdatedAt,
	}
	if withBlocks {
		out.Blocks = tr.Blocks
	}
	return out
}

func (s *ContentService) PublicPosts(locale, typ, term, q string, page, per int) ([]PublicPost, int64, error) {
	now := time.Now()
	tx := s.db.Model(&entity.Post{}).
		Where("status = ? AND (publish_at IS NULL OR publish_at <= ?) AND (unpublish_at IS NULL OR unpublish_at > ?)",
			entity.StatusPublished, now, now)
	if typ != "" {
		tx = tx.Where("type = ?", typ)
	}
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("lower(translations::text) LIKE ?", like)
	}
	if term != "" {
		ids, err := s.tax.TermPostIDs("", term)
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			return []PublicPost{}, 0, nil
		}
		tx = tx.Where("id IN ?", ids)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 50 {
		per = 12
	}
	var rows []entity.Post
	err := tx.Order("publish_at DESC").Limit(per).Offset((page-1)*per).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]PublicPost, 0, len(rows))
	for i := range rows {
		out = append(out, s.publicPostOf(&rows[i], locale, false))
	}
	return out, total, nil
}

func (s *ContentService) PublicPostBySlug(slug, locale string) (*PublicPost, error) {
	var p entity.Post
	err := s.db.First(&p, "slug = ?", slug).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !entity.LiveAt(p.Status, p.PublishAt, p.UnpublishAt, time.Now()) {
		return nil, ErrNotFound
	}
	out := s.publicPostOf(&p, locale, true)
	return &out, nil
}

type PublicBanner struct {
	ID          string    `json:"id"`
	ImageURL    string    `json:"image_url"`
	Link        string    `json:"link,omitempty"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Locale      string    `json:"locale"`
	SortOrder   int       `json:"sort_order"`
}

func (s *ContentService) PublicBanners(locale string) ([]PublicBanner, error) {
	var rows []entity.Banner
	if err := s.db.Order("sort_order ASC, created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	out := make([]PublicBanner, 0, len(rows))
	for i := range rows {
		b := &rows[i]
		if !b.BannerLive(now) {
			continue
		}
		tr, resolved := entity.TrResolve(entity.TrOf[entity.BannerTranslation](b.Translations), locale)
		out = append(out, PublicBanner{
			ID: b.ID, ImageURL: b.ImageURL, Link: b.Link,
			Title: tr.Title, Description: tr.Description, Locale: resolved, SortOrder: b.SortOrder,
		})
	}
	return out, nil
}

type PublicEvent struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Subtitle   string    `json:"subtitle,omitempty"`
	EventDate  *time.Time `json:"event_date,omitempty"`
	Month      string    `json:"month,omitempty"`
	Year       int       `json:"year,omitempty"`
	Location   string    `json:"location,omitempty"`
	HeroURL    string    `json:"hero_url,omitempty"`
	Locale     string    `json:"locale"`
}

func (s *ContentService) PublicEvents(locale string, page, per int) ([]PublicEvent, int64, error) {
	now := time.Now()
	tx := s.db.Model(&entity.Event{}).
		Where("status = ? AND (publish_at IS NULL OR publish_at <= ?) AND (unpublish_at IS NULL OR unpublish_at > ?)",
			entity.StatusPublished, now, now)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if per < 1 || per > 50 {
		per = 20
	}
	var rows []entity.Event
	err := tx.Order("event_date DESC").Limit(per).Offset((page-1)*per).Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	out := make([]PublicEvent, 0, len(rows))
	for i := range rows {
		e := &rows[i]
		tr, resolved := entity.TrResolve(entity.TrOf[entity.EventTranslation](e.Translations), locale)
		out = append(out, PublicEvent{
			Slug: e.Slug, Title: tr.Title, Subtitle: tr.Subtitle,
			EventDate: e.EventDate, Month: e.Month, Year: e.Year,
			Location: e.Location, HeroURL: e.HeroURL, Locale: resolved,
		})
	}
	return out, total, nil
}

type PublicPage struct {
	Slug     string         `json:"slug"`
	Template string         `json:"template,omitempty"`
	Title    string         `json:"title"`
	Blocks   []blocks.Block `json:"blocks,omitempty"`
	Locale   string         `json:"locale"`
	SEO      entity.SEO     `json:"seo,omitempty"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func (s *ContentService) PublicPageBySlug(slug, locale string) (*PublicPage, error) {
	var p entity.Page
	if err := s.db.First(&p, "slug = ?", slug).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if !entity.LiveAt(p.Status, p.PublishAt, p.UnpublishAt, time.Now()) {
		return nil, ErrNotFound
	}
	tr, resolved := entity.TrResolve(entity.TrOf[entity.PageTranslation](p.Translations), locale)
	return &PublicPage{
		Slug: p.Slug, Template: p.Template, Title: tr.Title, Blocks: tr.Blocks,
		Locale: resolved, SEO: entity.SEOOf(p.SEO), UpdatedAt: p.UpdatedAt,
	}, nil
}

func (s *ContentService) PublicDocuments() ([]entity.Document, error) {
	var rows []entity.Document
	err := s.db.Where("active = ?", true).
		Order("academic_year DESC, group_label ASC, sort_order ASC").Find(&rows).Error
	return rows, err
}

// ==================== helpers ====================

func marshalSEO(seo entity.SEO) string {
	b, err := json.Marshal(seo)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func indonesianMonth(m time.Month) string {
	names := map[time.Month]string{
		time.January: "Januari", time.February: "Februari", time.March: "Maret",
		time.April: "April", time.May: "Mei", time.June: "Juni",
		time.July: "Juli", time.August: "Agustus", time.September: "September",
		time.October: "Oktober", time.November: "November", time.December: "Desember",
	}
	return names[m]
}

// ==================== F2: VERSIONING + ROLLBACK (Post) ====================

// PostSnapshot — kondisi konten Post pada satu versi (field yang diedit editor).
type PostSnapshot struct {
	Slug          string                              `json:"slug"`
	Type          string                              `json:"type"`
	Featured      bool                                `json:"featured"`
	HeroURL       string                              `json:"hero_url"`
	HeroMediaID   *string                             `json:"hero_media_id"`
	PublishAt     *time.Time                          `json:"publish_at"`
	UnpublishAt   *time.Time                          `json:"unpublish_at"`
	Translations  map[string]entity.PostTranslation   `json:"translations"`
	SEO           entity.SEO                          `json:"seo"`
}

func postSnapshotOf(p *entity.Post) PostSnapshot {
	return PostSnapshot{
		Slug: p.Slug, Type: p.Type, Featured: p.Featured,
		HeroURL: p.HeroURL, HeroMediaID: p.HeroMediaID,
		PublishAt: p.PublishAt, UnpublishAt: p.UnpublishAt,
		Translations: entity.TrOf[entity.PostTranslation](p.Translations),
		SEO:          entity.SEOOf(p.SEO),
	}
}

// saveVersion — simpan snapshot versi (idempotent, dalam transaksi caller).
func (s *ContentService) saveVersion(tx *gorm.DB, entityType, entityID string, version int, snap any, actor string) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	cv := &entity.ContentVersion{
		EntityType: entityType, EntityID: entityID, Version: version,
		Snapshot: string(b), Actor: actor,
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(cv).Error
}

// VersionSummary — item riwayat versi utk UI.
type VersionSummary struct {
	Version     int       `json:"version"`
	Actor       string    `json:"actor"`
	CreatedAt   time.Time `json:"created_at"`
	Title       string    `json:"title"`
	IsPublished bool      `json:"is_published"`
}

func (s *ContentService) ListPostVersions(id string) ([]VersionSummary, error) {
	var p entity.Post
	if err := s.db.Select("id, published_version").First(&p, "id = ?", id).Error; err != nil {
		return nil, err
	}
	var rows []entity.ContentVersion
	if err := s.db.Where("entity_type = ? AND entity_id = ?", "post", id).Order("version DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]VersionSummary, 0, len(rows))
	for _, cv := range rows {
		vs := VersionSummary{Version: cv.Version, Actor: cv.Actor, CreatedAt: cv.CreatedAt, IsPublished: cv.Version == p.PublishedVersion}
		var snap PostSnapshot
		if json.Unmarshal([]byte(cv.Snapshot), &snap) == nil && snap.Translations != nil {
			vs.Title = snap.Translations["id"].Title
		}
		out = append(out, vs)
	}
	return out, nil
}

func (s *ContentService) GetPostVersion(id string, version int) (*PostSnapshot, error) {
	var cv entity.ContentVersion
	if err := s.db.Where("entity_type = ? AND entity_id = ? AND version = ?", "post", id, version).First(&cv).Error; err != nil {
		return nil, err
	}
	var snap PostSnapshot
	if err := json.Unmarshal([]byte(cv.Snapshot), &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// RollbackPost — pulihkan konten dari snapshot versi target (status TIDAK diubah;
// hasil rollback disimpan sebagai versi baru agar riwayat tetap utuh).
func (s *ContentService) RollbackPost(id string, targetVersion int, actor string) (*entity.Post, error) {
	var cv entity.ContentVersion
	if err := s.db.Where("entity_type = ? AND entity_id = ? AND version = ?", "post", id, targetVersion).First(&cv).Error; err != nil {
		return nil, err
	}
	var snap PostSnapshot
	if err := json.Unmarshal([]byte(cv.Snapshot), &snap); err != nil {
		return nil, err
	}
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	if snap.Slug != "" && snap.Slug != p.Slug {
		p.Slug = s.uniqueSlug(snap.Slug, "posts")
	}
	if snap.Type != "" {
		p.Type = snap.Type
	}
	p.Featured = snap.Featured
	p.HeroURL = snap.HeroURL
	p.HeroMediaID = snap.HeroMediaID
	p.PublishAt = snap.PublishAt
	p.UnpublishAt = snap.UnpublishAt
	p.SEO = marshalSEO(snap.SEO)
	p.Version++
	p.UpdatedBy = actor
	p.Translations = "{}"
	for loc, t := range snap.Translations {
		p.SetTr(loc, t)
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(p).Error; err != nil {
			return err
		}
		return s.saveVersion(tx, "post", p.ID, p.Version, postSnapshotOf(p), actor)
	}); err != nil {
		return nil, err
	}
	s.FlushPublicCache()
	return p, nil
}

// ==================== F2: EDIT LOCKING (Redis, TTL 10 mnt) ====================

type LockStatus struct {
	Locked    bool      `json:"locked"`
	Holder    string    `json:"holder,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

func lockKey(entityType, id string) string { return "bwm:lock:" + entityType + ":" + id }

// AcquirePostLock — klaim lock edit; holder sama = refresh TTL (heartbeat).
func (s *ContentService) AcquirePostLock(id, actor string, force bool) (LockStatus, bool, error) {
	if s.redis == nil {
		return LockStatus{}, true, nil // tanpa Redis: locking nonaktif, izinkan edit
	}
	ctx := context.Background()
	key := lockKey("post", id)
	ttl := 10 * time.Minute
	if force {
		if err := s.redis.Set(ctx, key, actor, ttl).Err(); err != nil {
			return LockStatus{}, false, err
		}
		return LockStatus{Locked: true, Holder: actor, ExpiresAt: time.Now().Add(ttl)}, true, nil
	}
	ok, err := s.redis.SetNX(ctx, key, actor, ttl).Result()
	if err != nil {
		return LockStatus{}, false, err
	}
	if ok {
		return LockStatus{Locked: true, Holder: actor, ExpiresAt: time.Now().Add(ttl)}, true, nil
	}
	holder, _ := s.redis.Get(ctx, key).Result()
	if holder == actor {
		_ = s.redis.Expire(ctx, key, ttl).Err()
		return LockStatus{Locked: true, Holder: actor, ExpiresAt: time.Now().Add(ttl)}, true, nil
	}
	return LockStatus{Locked: true, Holder: holder}, false, nil
}

// ReleasePostLock — lepaskan (hanya holder, atau force).
func (s *ContentService) ReleasePostLock(id, actor string, force bool) error {
	if s.redis == nil {
		return nil
	}
	ctx := context.Background()
	key := lockKey("post", id)
	holder, _ := s.redis.Get(ctx, key).Result()
	if force || holder == actor || holder == "" {
		return s.redis.Del(ctx, key).Err()
	}
	return errors.New("lock sedang dipegang editor lain")
}

// GetPostLock — status lock saat ini.
func (s *ContentService) GetPostLock(id string) (LockStatus, error) {
	if s.redis == nil {
		return LockStatus{}, nil
	}
	ctx := context.Background()
	holder, err := s.redis.Get(ctx, lockKey("post", id)).Result()
	if err == redis.Nil {
		return LockStatus{}, nil
	}
	if err != nil {
		return LockStatus{}, err
	}
	ttl, _ := s.redis.TTL(ctx, lockKey("post", id)).Result()
	return LockStatus{Locked: true, Holder: holder, ExpiresAt: time.Now().Add(ttl)}, nil
}

// ==================== F2: TAKSONOMI (publik) ====================

// PublicTerms — daftar term utk web (kategori/tag + jumlah konten).
func (s *ContentService) PublicTerms(taxonomy string) ([]TermWithCount, error) {
	return s.tax.ListTerms(taxonomy)
}

// PublicRelatedPosts — konten terkait (overlap term terbanyak, hanya tayang).
func (s *ContentService) PublicRelatedPosts(slug, locale string, limit int) ([]PublicPost, error) {
	var p entity.Post
	if err := s.db.Select("id, slug").First(&p, "slug = ?", slug).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ids, err := s.tax.RelatedPostIDs(p.ID, limit*3)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []PublicPost{}, nil
	}
	now := time.Now()
	var rows []entity.Post
	if err := s.db.Where("id IN ? AND status = ? AND (publish_at IS NULL OR publish_at <= ?) AND (unpublish_at IS NULL OR unpublish_at > ?)",
		ids, entity.StatusPublished, now, now).
		Order("publish_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]PublicPost, 0, len(rows))
	for i := range rows {
		out = append(out, s.publicPostOf(&rows[i], locale, false))
	}
	return out, nil
}

// ==================== F2: SEO SUITE (sitemap.xml + RSS) ====================

// publicBaseURL — basis URL web publik (env BWM_PUBLIC_BASE_URL, fallback produksi).
func publicBaseURL() string {
	if v := os.Getenv("BWM_PUBLIC_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://binawan.ac.id"
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// SitemapXML — sitemap konten tayang (posts -> /kabar-kampus/<slug>).
func (s *ContentService) SitemapXML() ([]byte, error) {
	now := time.Now()
	var rows []entity.Post
	err := s.db.Model(&entity.Post{}).
		Where("status = ? AND (publish_at IS NULL OR publish_at <= ?) AND (unpublish_at IS NULL OR unpublish_at > ?)",
			entity.StatusPublished, now, now).
		Select("slug, updated_at, publish_at").
		Order("updated_at DESC").Limit(5000).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	base := publicBaseURL()
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, p := range rows {
		lastmod := p.UpdatedAt
		if p.PublishAt != nil && p.PublishAt.After(lastmod) {
			lastmod = *p.PublishAt
		}
		fmt.Fprintf(&b, "  <url>\n    <loc>%s/kabar-kampus/%s</loc>\n    <lastmod>%s</lastmod>\n  </url>\n",
			base, xmlEscape(p.Slug), lastmod.UTC().Format("2006-01-02"))
	}
	b.WriteString("</urlset>")
	return []byte(b.String()), nil
}

// RSSFeed — RSS 2.0 kabar kampus (20 tayang terbaru).
func (s *ContentService) RSSFeed() ([]byte, error) {
	now := time.Now()
	var rows []entity.Post
	err := s.db.Model(&entity.Post{}).
		Where("status = ? AND (publish_at IS NULL OR publish_at <= ?) AND (unpublish_at IS NULL OR unpublish_at > ?)",
			entity.StatusPublished, now, now).
		Order("publish_at DESC").Limit(20).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	base := publicBaseURL()
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<rss version=\"2.0\"><channel>\n")
	fmt.Fprintf(&b, "  <title>Kabar Kampus Universitas Binawan</title>\n  <link>%s/kabar-kampus</link>\n  <description>Berita dan kabar terbaru Universitas Binawan</description>\n", base)
	fmt.Fprintf(&b, "  <lastBuildDate>%s</lastBuildDate>\n", time.Now().UTC().Format(time.RFC1123Z))
	for i := range rows {
		p := &rows[i]
		tr := entity.TrOf[entity.PostTranslation](p.Translations)["id"]
		pub := time.Now()
		if p.PublishAt != nil {
			pub = *p.PublishAt
		}
		fmt.Fprintf(&b, "  <item>\n    <title>%s</title>\n    <link>%s/kabar-kampus/%s</link>\n    <guid>%s/kabar-kampus/%s</guid>\n    <pubDate>%s</pubDate>\n    <description>%s</description>\n  </item>\n",
			xmlEscape(tr.Title), base, xmlEscape(p.Slug), base, xmlEscape(p.Slug),
			pub.UTC().Format(time.RFC1123Z), xmlEscape(tr.Excerpt))
	}
	b.WriteString("</channel></rss>")
	return []byte(b.String()), nil
}

// ==================== F2: EXPORT JSON PENUH (asuransi data) ====================

// ExportAll — snapshot seluruh konten CMS sbg map siap-JSON.
// Versioning internal (content_versions) tidak disertakan — hanya data inti.
func (s *ContentService) ExportAll() (map[string]any, error) {
	var posts []entity.Post
	if err := s.db.Order("updated_at DESC").Find(&posts).Error; err != nil {
		return nil, err
	}
	s.tax.PopulateTerms(posts)

	var pages []entity.Page
	if err := s.db.Order("updated_at DESC").Find(&pages).Error; err != nil {
		return nil, err
	}
	var events []entity.Event
	if err := s.db.Order("event_date DESC").Find(&events).Error; err != nil {
		return nil, err
	}
	var banners []entity.Banner
	if err := s.db.Order("sort_order").Find(&banners).Error; err != nil {
		return nil, err
	}
	var terms []entity.Term
	if err := s.db.Order("taxonomy, slug").Find(&terms).Error; err != nil {
		return nil, err
	}
	var postTerms []entity.PostTerm
	if err := s.db.Find(&postTerms).Error; err != nil {
		return nil, err
	}
	var media []entity.MediaAsset
	if err := s.db.Order("created_at DESC").Find(&media).Error; err != nil {
		return nil, err
	}
	var documents []entity.Document
	if err := s.db.Order("created_at DESC").Find(&documents).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"format":      "bwm-export/1",
		"data": map[string]any{
			"posts": posts, "pages": pages, "events": events, "banners": banners,
			"terms": terms, "post_terms": postTerms,
			"media": media, "documents": documents,
		},
	}, nil
}

// ==================== F2: REDIRECT MANAGER ====================

// ListRedirects — daftar semua pemetaan (opsional filter aktif).
func (s *ContentService) ListRedirects(activeOnly bool) ([]entity.Redirect, error) {
	q := s.db.Model(&entity.Redirect{})
	if activeOnly {
		q = q.Where("active = ?", true)
	}
	var rows []entity.Redirect
	err := q.Order("created_at DESC").Find(&rows).Error
	return rows, err
}

// CreateRedirect — tambah pemetaan (from_path dinormalkan: harus diawali "/").
func (s *ContentService) CreateRedirect(fromPath, toPath string, statusCode int, note, actor string) (*entity.Redirect, error) {
	if !strings.HasPrefix(fromPath, "/") || !strings.HasPrefix(toPath, "/") {
		return nil, fmt.Errorf("from/to path harus diawali \"/\": %w", ErrInvalid)
	}
	if statusCode != 301 && statusCode != 302 {
		statusCode = 301
	}
	r := &entity.Redirect{
		FromPath: strings.TrimSpace(fromPath), ToPath: strings.TrimSpace(toPath),
		StatusCode: statusCode, Active: true, Note: note, CreatedBy: actor,
	}
	if err := s.db.Create(r).Error; err != nil {
		return nil, err
	}
	return r, nil
}

// UpdateRedirect — ubah target/status/aktif.
func (s *ContentService) UpdateRedirect(id, toPath string, statusCode int, active bool) (*entity.Redirect, error) {
	var r entity.Redirect
	if err := s.db.First(&r, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if strings.HasPrefix(toPath, "/") {
		r.ToPath = strings.TrimSpace(toPath)
	}
	if statusCode == 301 || statusCode == 302 {
		r.StatusCode = statusCode
	}
	r.Active = active
	if err := s.db.Save(&r).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// DeleteRedirect — hapus pemetaan.
func (s *ContentService) DeleteRedirect(id string) error {
	return s.db.Delete(&entity.Redirect{}, "id = ?", id).Error
}

// LookupRedirect — cari tujuan utk path (publik; hanya yang aktif).
func (s *ContentService) LookupRedirect(fromPath string) (*entity.Redirect, error) {
	var r entity.Redirect
	err := s.db.Where("from_path = ? AND active = ?", strings.TrimSpace(fromPath), true).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ensurePostRedirect — saat slug post berubah: otomatis pasang redirect 301
// dari path lama ke baru (dipanggil UpdatePost).
func (s *ContentService) ensurePostRedirect(oldSlug, newSlug string, actor string) {
	if oldSlug == "" || oldSlug == newSlug {
		return
	}
	from := "/kabar-kampus/" + oldSlug
	to := "/kabar-kampus/" + newSlug
	_, _ = s.CreateRedirect(from, to, 301, "otomatis: slug berubah", actor)
}

// ==================== F3: REVIEW WORKFLOW ====================

// SubmitForReview — draft -> in_review (menunggu persetujuan editor).
func (s *ContentService) SubmitForReview(id, actor string) (*entity.Post, error) {
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	if p.Status != entity.StatusDraft {
		return nil, fmt.Errorf("hanya draft yang bisa dikirim review (status saat ini: %s): %w", p.Status, ErrInvalid)
	}
	p.Status = entity.StatusInReview
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
	}
	_ = s.db.Create(&entity.ContentReview{EntityType: "post", EntityID: p.ID, Action: "submit", Actor: actor}).Error
	return p, nil
}

// ApprovePost — in_review -> published (setara publish, dengan jejak review).
func (s *ContentService) ApprovePost(id, actor string) (*entity.Post, error) {
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	if p.Status != entity.StatusInReview {
		return nil, fmt.Errorf("approve hanya untuk konten menunggu review (status: %s): %w", p.Status, ErrInvalid)
	}
	now := time.Now()
	p.Status = entity.StatusPublished
	p.PublishAt = &now
	p.PublishedVersion = p.Version
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
	}
	_ = s.db.Create(&entity.ContentReview{EntityType: "post", EntityID: p.ID, Action: "approve", Actor: actor}).Error
	s.FlushPublicCache()
	FireWebhooks(s.webhooks, PublishEvent{Event: "publish", Entity: "post", ID: p.ID, Slug: p.Slug, At: now})
	return p, nil
}

// RejectPost — in_review -> draft + catatan wajib (kembali ke penulis).
func (s *ContentService) RejectPost(id, actor, note string) (*entity.Post, error) {
	if strings.TrimSpace(note) == "" {
		return nil, fmt.Errorf("catatan penolakan wajib diisi: %w", ErrInvalid)
	}
	p, err := s.GetPost(id)
	if err != nil {
		return nil, err
	}
	if p.Status != entity.StatusInReview {
		return nil, fmt.Errorf("reject hanya untuk konten menunggu review (status: %s): %w", p.Status, ErrInvalid)
	}
	p.Status = entity.StatusDraft
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
	}
	_ = s.db.Create(&entity.ContentReview{EntityType: "post", EntityID: p.ID, Action: "reject", Note: note, Actor: actor}).Error
	return p, nil
}

// ListReviews — jejak alur editorial sebuah konten (terbaru dulu).
func (s *ContentService) ListReviews(id string) ([]entity.ContentReview, error) {
	var rows []entity.ContentReview
	err := s.db.Where("entity_type = ? AND entity_id = ?", "post", id).
		Order("created_at DESC").Limit(50).Find(&rows).Error
	return rows, err
}

// ==================== F3: SCHEDULED PUBLISH/UNPUBLISH (cron per menit) ====================

// StartScheduler — loop per menit: terbitkan konten terjadwal yang jatuh tempo,
// turunkan konten tayang yang lewat unpublish_at. Jalankan sbg goroutine.
func (s *ContentService) StartScheduler(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tickScheduler()
		}
	}
}

func (s *ContentService) tickScheduler() {
	now := time.Now()
	var due []entity.Post
	if err := s.db.Where("status = ? AND publish_at IS NOT NULL AND publish_at <= ?",
		entity.StatusScheduled, now).Find(&due).Error; err != nil {
		return
	}
	for i := range due {
		p := &due[i]
		p.Status = entity.StatusPublished
		p.PublishedVersion = p.Version
		if err := s.db.Save(p).Error; err != nil {
			continue
		}
		log.Printf("[scheduler] publish otomatis: %s (%s)", p.Slug, p.ID)
		s.FlushPublicCache()
		FireWebhooks(s.webhooks, PublishEvent{Event: "publish", Entity: "post", ID: p.ID, Slug: p.Slug, At: time.Now()})
	}
	var expire []entity.Post
	if err := s.db.Where("status = ? AND unpublish_at IS NOT NULL AND unpublish_at <= ?",
		entity.StatusPublished, now).Find(&expire).Error; err != nil {
		return
	}
	for i := range expire {
		p := &expire[i]
		p.Status = entity.StatusDraft
		if err := s.db.Save(p).Error; err != nil {
			continue
		}
		log.Printf("[scheduler] unpublish otomatis: %s (%s)", p.Slug, p.ID)
		s.FlushPublicCache()
		FireWebhooks(s.webhooks, PublishEvent{Event: "unpublish", Entity: "post", ID: p.ID, Slug: p.Slug, At: time.Now()})
	}
}
