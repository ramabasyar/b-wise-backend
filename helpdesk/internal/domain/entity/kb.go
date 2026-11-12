package entity

import "time"

// KBArticle — artikel basis pengetahuan milik satu tim (KB-01, blueprint §6.9).
// Draft → terbit; setiap penyimpanan menaikkan versi.
type KBArticle struct {
	Base
	TeamID      string     `json:"team_id" gorm:"type:varchar(36);index;not null"`
	CollectionID *string    `json="collection_id,omitempty" gorm="type:varchar(36);index"`
	ParentID     *string    `json="parent_id,omitempty" gorm="type:varchar(36);index"`
	Slug        string     `json:"slug,omitempty" gorm:"type:varchar(160);index"`
	Title       string     `json:"title" gorm:"type:varchar(200);not null"`
	Body        string     `json:"body,omitempty" gorm:"type:text"`
	Status      string     `json:"status" gorm:"type:varchar(20);default:'draft';index"`
	Visibility  string     `json:"visibility" gorm="type:varchar(20);default:""public"";index"`
	Version     int        `json:"version" gorm:"default:1"`
	Audience    string     `json:"audience,omitempty" gorm:"type:jsonb"`
	Helpful     int        `json:"helpful" gorm:"default:0"`
	NotHelpful  int        `json:"not_helpful" gorm:"default:0"`
	CreatedBy   string     `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	PublishedAt *time.Time `json:"published_at,omitempty" gorm:"type:timestamptz"`
}

func (KBArticle) TableName() string { return "kb_articles" }

// KBArticleFeedback — satu umpan balik per user per artikel (KB-05).
type KBArticleFeedback struct {
	Base
	ArticleID string `json:"article_id" gorm:"type:varchar(36);uniqueIndex:idx_kb_fb;not null"`
	UserID    string `json:"user_id" gorm:"type:varchar(36);uniqueIndex:idx_kb_fb;not null"`
	Helpful   bool   `json:"helpful"`
}

func (KBArticleFeedback) TableName() string { return "kb_article_feedbacks" }


// KBCollection - kategori/koleksi artikel KB per tim (fase 2 wiki).
type KBCollection struct {
	Base
	TeamID      string `json="team_id" gorm="type:varchar(36);index;not null"`
	Name        string `json="name" gorm="type:varchar(120);not null"`
	Slug        string `json="slug,omitempty" gorm="type:varchar(140);index"`
	Description string `json="description,omitempty" gorm="type:text"`
	SortOrder   int    `json="sort_order" gorm="default:0"`
}
func (KBCollection) TableName() string { return "kb_collections" }

// KBRevision - snapshot isi artikel tiap update; restore membuat versi baru.
type KBRevision struct {
	Base
	ArticleID string `json="article_id" gorm="type:varchar(36);index;not null"`
	Version   int    `json="version" gorm="not null"`
	Title     string `json="title" gorm="type:varchar(200);not null"`
	Body      string `json="body" gorm="type:text"`
	EditedBy  string `json="edited_by,omitempty" gorm="type:varchar(36)"`
}
func (KBRevision) TableName() string { return "kb_revisions" }

// KBImage - gambar artikel KB (fase 3); object key relatif root attachment.
type KBImage struct {
	Base
	TeamID    string `json="team_id" gorm="type:varchar(36);index;not null"`
	ObjectKey string `json="-" gorm="type:varchar(300);not null"`
	Filename  string `json="filename" gorm="type:varchar(255);not null"`
	Mime      string `json="mime,omitempty" gorm="type:varchar(100)"`
	Size      int64  `json="size" gorm="default:0"`
}
func (KBImage) TableName() string { return "kb_images" }