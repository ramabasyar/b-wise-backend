package entity

import "time"

// KBArticle — artikel basis pengetahuan milik satu tim (KB-01, blueprint §6.9).
// Draft → terbit; setiap penyimpanan menaikkan versi.
type KBArticle struct {
	Base
	TeamID      string     `json:"team_id" gorm:"type:varchar(36);index;not null"`
	Slug        string     `json:"slug,omitempty" gorm:"type:varchar(160);index"`
	Title       string     `json:"title" gorm:"type:varchar(200);not null"`
	Body        string     `json:"body,omitempty" gorm:"type:text"`
	Status      string     `json:"status" gorm:"type:varchar(20);default:'draft';index"`
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
