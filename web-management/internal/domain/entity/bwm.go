package entity

import (
	"encoding/json"
	"time"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
)

// ==================== BWM — Binawan Web Management (F0) ====================
// Enitas konten inti. Pola umum:
//   - lifecycle: status + publish_at/unpublish_at + version/published_version
//   - dwibahasa: kolom translations jsonb {id:{...}, en:{...}} (fallback id)
//   - audit: created_by/updated_by (user SSO)

const (
	StatusDraft     = "draft"
	StatusInReview  = "in_review"
	StatusScheduled = "scheduled"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

var LiveStatuses = []string{StatusPublished}

// LiveAt — konten "hidup" bila published DAN dalam jendela tayang.
func LiveAt(status string, publishAt, unpublishAt *time.Time, now time.Time) bool {
	if status != StatusPublished {
		return false
	}
	if publishAt != nil && now.Before(*publishAt) {
		return false
	}
	if unpublishAt != nil && !now.Before(*unpublishAt) {
		return false
	}
	return true
}

// ---------- tipe terjemahkan ----------

// PostTranslation — field per-locale utk Post.
type PostTranslation struct {
	Title    string         `json:"title,omitempty"`
	Excerpt  string         `json:"excerpt,omitempty"`
	Author   string         `json:"author,omitempty"`
	Category string         `json:"category,omitempty"`
	HeroAlt  string         `json:"hero_alt,omitempty"`
	Blocks   []blocks.Block `json:"blocks,omitempty"`
}

// PageTranslation — field per-locale utk Page.
type PageTranslation struct {
	Title string         `json:"title,omitempty"`
	Blocks []blocks.Block `json:"blocks,omitempty"`
}

// EventTranslation — field per-locale utk Event.
type EventTranslation struct {
	Title       string `json:"title,omitempty"`
	Subtitle    string `json:"subtitle,omitempty"`
	Description string `json:"description,omitempty"`
}

// BannerTranslation — field per-locale utk Banner.
type BannerTranslation struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// SEO — metadata SEO per konten.
type SEO struct {
	MetaTitle       string `json:"meta_title,omitempty"`
	MetaDescription string `json:"meta_description,omitempty"`
	OGImage         string `json:"og_image,omitempty"`
	Canonical       string `json:"canonical,omitempty"`
}

// ---------- helpers jsonb ----------

func marshalJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// TrOf — decode kolom translations jsonb → map[locale]T (aman utk kosong/rousak).
func TrOf[T any](raw string) map[string]T {
	m := map[string]T{}
	if raw == "" {
		return m
	}
	_ = json.Unmarshal([]byte(raw), &m)
	return m
}

// TrResolve — ambil locale diminta dgn fallback "id".
func TrResolve[T any](m map[string]T, locale string) (t T, resolved string) {
	if locale == "" {
		locale = "id"
	}
	if v, ok := m[locale]; ok {
		return v, locale
	}
	if v, ok := m["id"]; ok {
		return v, "id"
	}
	return t, ""
}

// SEOOf — decode kolom seo jsonb.
func SEOOf(raw string) SEO {
	s := SEO{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &s)
	}
	return s
}

// ==================== Post ====================
// Kabar kampus / artikel. Menyerap KabarKampus + KabarTerbaru (+ artikel nanti).

type Post struct {
	Base
	Slug             string     `json:"slug" gorm:"type:varchar(220);uniqueIndex;not null"`
	Type             string     `json:"type" gorm:"type:varchar(20);default:'news';index"` // news|article
	Status           string     `json:"status" gorm:"type:varchar(20);default:'draft';index"`
	Featured         bool       `json:"featured" gorm:"default:false"`
	HeroURL          string     `json:"hero_url" gorm:"type:varchar(500)"` // F0: path/URL legacy; naik ke MediaAsset saat MinIO aktif
	HeroMediaID      *string    `json:"hero_media_id" gorm:"type:varchar(36)"`
	Source           string     `json:"source" gorm:"type:varchar(20);default:'manual'"` // manual|import
	PublishAt        *time.Time `json:"publish_at"`
	UnpublishAt      *time.Time `json:"unpublish_at"`
	Translations     string     `json:"translations" gorm:"type:jsonb;default:'{}'"`
	SEO              string     `json:"seo" gorm:"type:jsonb;default:'{}'"`
	Version          int        `json:"version" gorm:"default:1"`
	PublishedVersion int        `json:"published_version" gorm:"default:0"`
	CreatedBy        string     `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy        string     `json:"updated_by" gorm:"type:varchar(36)"`
}

func (Post) TableName() string { return "posts" }

// MarshalJSON — translations & seo dikirim sbg OBJEK ter-parse (bukan string jsonb
// mentah) supaya UI editor & konsumen API tinggal pakai.
func (p Post) MarshalJSON() ([]byte, error) {
	type Alias Post
	tr := map[string]PostTranslation{}
	if p.Translations != "" {
		_ = json.Unmarshal([]byte(p.Translations), &tr)
	}
	return json.Marshal(struct {
		Alias
		Translations map[string]PostTranslation `json:"translations"`
		SEO          SEO                        `json:"seo"`
	}{Alias(p), tr, SEOOf(p.SEO)})
}

// SetTr — simpan satu locale (marshal seluruh map).
func (p *Post) SetTr(locale string, t PostTranslation) {
	m := TrOf[PostTranslation](p.Translations)
	if m == nil {
		m = map[string]PostTranslation{}
	}
	m[locale] = t
	p.Translations = marshalJSON(m)
}

// ==================== Page ====================
// Halaman statis (profil, LPM, fasilitas, dst.) — konten blocks per locale.

type Page struct {
	Base
	Slug             string     `json:"slug" gorm:"type:varchar(220);uniqueIndex;not null"`
	ParentID         *string    `json:"parent_id" gorm:"type:varchar(36);index"`
	Template         string     `json:"template" gorm:"type:varchar(50);default:'default'"`
	Status           string     `json:"status" gorm:"type:varchar(20);default:'draft';index"`
	PublishAt        *time.Time `json:"publish_at"`
	UnpublishAt      *time.Time `json:"unpublish_at"`
	Translations     string     `json:"translations" gorm:"type:jsonb;default:'{}'"`
	SEO              string     `json:"seo" gorm:"type:jsonb;default:'{}'"`
	Version          int        `json:"version" gorm:"default:1"`
	PublishedVersion int        `json:"published_version" gorm:"default:0"`
	CreatedBy        string     `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy        string     `json:"updated_by" gorm:"type:varchar(36)"`
}

func (Page) TableName() string { return "pages" }

func (p Page) MarshalJSON() ([]byte, error) {
	type Alias Page
	tr := map[string]PageTranslation{}
	if p.Translations != "" {
		_ = json.Unmarshal([]byte(p.Translations), &tr)
	}
	return json.Marshal(struct {
		Alias
		Translations map[string]PageTranslation `json:"translations"`
		SEO          SEO                        `json:"seo"`
	}{Alias(p), tr, SEOOf(p.SEO)})
}

func (p *Page) SetTr(locale string, t PageTranslation) {
	m := TrOf[PageTranslation](p.Translations)
	if m == nil {
		m = map[string]PageTranslation{}
	}
	m[locale] = t
	p.Translations = marshalJSON(m)
}

// ==================== Event ====================
// Agenda/acara kampus (menyerap EventKampus).

type Event struct {
	Base
	Slug             string     `json:"slug" gorm:"type:varchar(220);uniqueIndex;not null"`
	Status           string     `json:"status" gorm:"type:varchar(20);default:'draft';index"`
	EventDate        *time.Time `json:"event_date" gorm:"index"`
	Month            string     `json:"month" gorm:"type:varchar(20)"`
	Year             int        `json:"year"`
	Location         string     `json:"location" gorm:"type:varchar(300)"`
	HeroURL          string     `json:"hero_url" gorm:"type:varchar(500)"`
	HeroMediaID      *string    `json:"hero_media_id" gorm:"type:varchar(36)"`
	PublishAt        *time.Time `json:"publish_at"`
	UnpublishAt      *time.Time `json:"unpublish_at"`
	Translations     string     `json:"translations" gorm:"type:jsonb;default:'{}'"`
	SEO              string     `json:"seo" gorm:"type:jsonb;default:'{}'"`
	Version          int        `json:"version" gorm:"default:1"`
	PublishedVersion int        `json:"published_version" gorm:"default:0"`
	CreatedBy        string     `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy        string     `json:"updated_by" gorm:"type:varchar(36)"`
}

func (Event) TableName() string { return "events" }

func (e Event) MarshalJSON() ([]byte, error) {
	type Alias Event
	tr := map[string]EventTranslation{}
	if e.Translations != "" {
		_ = json.Unmarshal([]byte(e.Translations), &tr)
	}
	return json.Marshal(struct {
		Alias
		Translations map[string]EventTranslation `json:"translations"`
		SEO          SEO                        `json:"seo"`
	}{Alias(e), tr, SEOOf(e.SEO)})
}

func (e *Event) SetTr(locale string, t EventTranslation) {
	m := TrOf[EventTranslation](e.Translations)
	if m == nil {
		m = map[string]EventTranslation{}
	}
	m[locale] = t
	e.Translations = marshalJSON(m)
}

// ==================== Banner ====================
// Slider home (menyerap Slider.json) — tanpa slug, diurutkan sort_order.

type Banner struct {
	Base
	ImageURL         string     `json:"image_url" gorm:"type:varchar(500)"`
	ImageMediaID     *string    `json:"image_media_id" gorm:"type:varchar(36)"`
	Link             string     `json:"link" gorm:"type:varchar(500)"`
	SortOrder        int        `json:"sort_order" gorm:"default:0;index"`
	Active           bool       `json:"active" gorm:"default:true;index"`
	StartAt          *time.Time `json:"start_at"`
	EndAt            *time.Time `json:"end_at"`
	Translations     string     `json:"translations" gorm:"type:jsonb;default:'{}'"`
	CreatedBy        string     `json:"created_by" gorm:"type:varchar(36)"`
	UpdatedBy        string     `json:"updated_by" gorm:"type:varchar(36)"`
}

func (Banner) TableName() string { return "banners" }

func (b Banner) MarshalJSON() ([]byte, error) {
	type Alias Banner
	tr := map[string]BannerTranslation{}
	if b.Translations != "" {
		_ = json.Unmarshal([]byte(b.Translations), &tr)
	}
	return json.Marshal(struct {
		Alias
		Translations map[string]BannerTranslation `json:"translations"`
	}{Alias(b), tr})
}

func (b *Banner) SetTr(locale string, t BannerTranslation) {
	m := TrOf[BannerTranslation](b.Translations)
	if m == nil {
		m = map[string]BannerTranslation{}
	}
	m[locale] = t
	b.Translations = marshalJSON(m)
}

// BannerLive — aktif + dalam periode tayang.
func (b *Banner) BannerLive(now time.Time) bool {
	if !b.Active {
		return false
	}
	if b.StartAt != nil && now.Before(*b.StartAt) {
		return false
	}
	if b.EndAt != nil && !now.Before(*b.EndAt) {
		return false
	}
	return true
}

// ==================== MediaAsset ====================
// Aset media (MinIO) — varian ukuran dibuat otomatis saat upload (F1 MinIO).

type MediaVariant struct {
	Key    string `json:"key,omitempty"` // object key MinIO (utk delete); URL utk konsumen
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

type MediaAsset struct {
	Base
	FileKey    string                   `json:"file_key" gorm:"type:varchar(500)"` // object key di MinIO
	URL        string                   `json:"url" gorm:"type:varchar(500)"`
	Filename   string                   `json:"filename" gorm:"type:varchar(300)"`
	MIME       string                   `json:"mime" gorm:"type:varchar(100)"`
	Width      int                      `json:"width"`
	Height     int                      `json:"height"`
	SizeBytes  int64                    `json:"size_bytes"`
	Variants   string                   `json:"variants" gorm:"type:jsonb;default:'{}'"` // {thumb,medium,large: MediaVariant}
	Alt        string                   `json:"alt" gorm:"type:varchar(500)"`
	Caption    string                   `json:"caption" gorm:"type:varchar(500)"`
	Credit     string                   `json:"credit" gorm:"type:varchar(200)"`
	UploadedBy string                   `json:"uploaded_by" gorm:"type:varchar(36)"`
}

func (MediaAsset) TableName() string { return "media_assets" }

func (m *MediaAsset) VariantsMap() map[string]MediaVariant {
	return TrOf[MediaVariant](m.Variants)
}

// MarshalJSON — variants dikirim sbg OBJEK (hasil parse jsonb), bukan string mentah —
// konsumen API (web) tinggal pakai tanpa parse ganda.
func (m MediaAsset) MarshalJSON() ([]byte, error) {
	type Alias MediaAsset
	v := map[string]MediaVariant{}
	if m.Variants != "" {
		_ = json.Unmarshal([]byte(m.Variants), &v)
	}
	return json.Marshal(struct {
		Alias
		Variants map[string]MediaVariant `json:"variants"`
	}{Alias(m), v})
}

// ==================== Document ====================
// Dokumen unduhan (menyerap documentsData: SK, kalender, form).

type Document struct {
	Base
	Name         string `json:"name" gorm:"type:varchar(300);not null"`
	AcademicYear string `json:"academic_year" gorm:"type:varchar(20);index"`
	GroupLabel   string `json:"group_label" gorm:"type:varchar(100);index"`
	SortOrder    int    `json:"sort_order" gorm:"default:0"`
	FileURL      string `json:"file_url" gorm:"type:varchar(500)"`
	FileKey      string `json:"file_key" gorm:"type:varchar(500)"` // MinIO (menyusul)
	FileType     string `json:"file_type" gorm:"type:varchar(20)"` // pdf|doc|xls|…
	Active       bool   `json:"active" gorm:"default:true"`
}

func (Document) TableName() string { return "documents" }
