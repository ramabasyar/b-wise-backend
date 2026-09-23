package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	webhooks  []string
}

func NewContentService(db *gorm.DB, r *redis.Client) *ContentService {
	return &ContentService{db: db, redis: r}
}

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
		tx = tx.Where("lower(translations) LIKE ?", like)
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
	p.Status = entity.StatusPublished
	p.PublishAt = at
	p.PublishedVersion = p.Version
	p.UpdatedBy = actor
	if err := s.db.Save(p).Error; err != nil {
		return nil, err
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

func (s *ContentService) PublicPosts(locale, typ string, page, per int) ([]PublicPost, int64, error) {
	now := time.Now()
	tx := s.db.Model(&entity.Post{}).
		Where("status = ? AND (publish_at IS NULL OR publish_at <= ?) AND (unpublish_at IS NULL OR unpublish_at > ?)",
			entity.StatusPublished, now, now)
	if typ != "" {
		tx = tx.Where("type = ?", typ)
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
