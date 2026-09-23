package service

// TaksonomiService — kategori & tag lintas konten (F2: taksonomi).
// Term: taxonomy=category|tag, dwibahasa (name_id/name_en), slug unik per taxonomy.

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

type TaxonomyService struct {
	db *gorm.DB
}

func NewTaxonomyService(db *gorm.DB) *TaxonomyService {
	return &TaxonomyService{db: db}
}

// EnsureSeed — idempotent: kalau terms kosong, seed kategori dari posts lama
// (translations.id.category / en.category) + pasang relasinya.
func (s *TaxonomyService) EnsureSeed() error {
	var n int64
	if err := s.db.Model(&entity.Term{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	var posts []entity.Post
	if err := s.db.Find(&posts).Error; err != nil {
		return err
	}
	type catInfo struct{ id, en string }
	cats := map[string]*catInfo{}
	postCats := map[string][]string{}
	for i := range posts {
		tr := entity.TrOf[entity.PostTranslation](posts[i].Translations)
		cn := strings.TrimSpace(tr["id"].Category)
		en := strings.TrimSpace(tr["en"].Category)
		if cn == "" && en == "" {
			continue
		}
		key := cn
		if key == "" {
			key = en
		}
		if cats[key] == nil {
			cats[key] = &catInfo{id: cn, en: en}
		}
		postCats[posts[i].ID] = append(postCats[posts[i].ID], key)
	}
	if len(cats) == 0 {
		return nil
	}
	termsByName := map[string]*entity.Term{}
	for name, ci := range cats {
		t := &entity.Term{Taxonomy: "category", Slug: Slugify(name)}
		t.NameID = ci.id
		if t.NameID == "" {
			t.NameID = name
		}
		t.NameEN = ci.en
		if t.NameEN == "" {
			t.NameEN = name
		}
		if err := s.db.Create(t).Error; err != nil {
			return err
		}
		termsByName[name] = t
	}
	for postID, names := range postCats {
		for _, name := range names {
			if t := termsByName[name]; t != nil {
				_ = s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entity.PostTerm{PostID: postID, TermID: t.ID}).Error
			}
		}
	}
	return nil
}

// TermWithCount — term + jumlah post terpasang.
type TermWithCount struct {
	entity.TermBrief
	PostCount int64 `json:"post_count"`
}

func (s *TaxonomyService) ListTerms(taxonomy string) ([]TermWithCount, error) {
	q := s.db.Table("terms t").
		Select("t.id, t.taxonomy, t.slug, t.name_id, t.name_en, count(pt.post_id) AS post_count").
		Joins("LEFT JOIN post_terms pt ON pt.term_id = t.id")
	if taxonomy != "" {
		q = q.Where("t.taxonomy = ?", taxonomy)
	}
	var rows []TermWithCount
	err := q.Group("t.id, t.taxonomy, t.slug, t.name_id, t.name_en").Order("t.taxonomy, t.name_id").Find(&rows).Error
	return rows, err
}

func (s *TaxonomyService) CreateTerm(taxonomy, slug, nameID, nameEN string) (*entity.Term, error) {
	if taxonomy != "category" && taxonomy != "tag" {
		return nil, errors.New("taxonomy harus category atau tag")
	}
	if strings.TrimSpace(nameID) == "" {
		return nil, errors.New("nama wajib diisi")
	}
	if slug == "" {
		slug = Slugify(nameID)
	}
	t := &entity.Term{Taxonomy: taxonomy, Slug: slug, NameID: nameID, NameEN: nameEN}
	if err := s.db.Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TaxonomyService) UpdateTerm(id, nameID, nameEN string) (*entity.Term, error) {
	var t entity.Term
	if err := s.db.First(&t, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(nameID) != "" {
		t.NameID = nameID
	}
	if nameEN != "" {
		t.NameEN = nameEN
	}
	if err := s.db.Save(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *TaxonomyService) DeleteTerm(id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("term_id = ?", id).Delete(&entity.PostTerm{}).Error; err != nil {
			return err
		}
		return tx.Delete(&entity.Term{}, "id = ?", id).Error
	})
}

// FindOrCreateByName — untuk tag chips di UI (paket slug, buat bila belum).
func (s *TaxonomyService) FindOrCreateByName(taxonomy, name string) (*entity.Term, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("nama kosong")
	}
	slug := Slugify(name)
	var t entity.Term
	err := s.db.Where("taxonomy = ? AND slug = ?", taxonomy, slug).First(&t).Error
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	t = entity.Term{Taxonomy: taxonomy, Slug: slug, NameID: name, NameEN: name}
	if err := s.db.Create(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// SetPostTerms — ganti seluruh relasi term milik post (replace all).
func (s *TaxonomyService) SetPostTerms(postID string, termIDs []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("post_id = ?", postID).Delete(&entity.PostTerm{}).Error; err != nil {
			return err
		}
		for _, tid := range termIDs {
			if tid == "" {
				continue
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entity.PostTerm{PostID: postID, TermID: tid}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// PopulateTerms — isi field p.Terms untuk kumpulan post (dipanggil ContentService).
func (s *TaxonomyService) PopulateTerms(posts []entity.Post) {
	if len(posts) == 0 {
		return
	}
	ids := make([]string, len(posts))
	for i := range posts {
		ids[i] = posts[i].ID
	}
	var rows []struct {
		entity.TermBrief
		PostID string
	}
	if err := s.db.Table("post_terms pt").
		Select("t.id, t.taxonomy, t.slug, t.name_id, t.name_en, pt.post_id").
		Joins("JOIN terms t ON t.id = pt.term_id").
		Where("pt.post_id IN ?", ids).
		Order("t.taxonomy, t.slug").
		Scan(&rows).Error; err != nil {
		return
	}
	byPost := map[string][]entity.TermBrief{}
	for _, r := range rows {
		byPost[r.PostID] = append(byPost[r.PostID], r.TermBrief)
	}
	for i := range posts {
		posts[i].Terms = byPost[posts[i].ID]
	}
}

// PopulateTermsOne — isi Terms untuk satu post (pointer).
func (s *TaxonomyService) PopulateTermsOne(p *entity.Post) {
	posts := []entity.Post{*p}
	s.PopulateTerms(posts)
	p.Terms = posts[0].Terms
}

// TermPostIDs — daftar post_id untuk sebuah term (taxonomy+slug).
func (s *TaxonomyService) TermPostIDs(taxonomy, slug string) ([]string, error) {
	var ids []string
	q := s.db.Table("post_terms pt").
		Joins("JOIN terms t ON t.id = pt.term_id").
		Where("t.slug = ?", slug)
	if taxonomy != "" {
		q = q.Where("t.taxonomy = ?", taxonomy)
	}
	err := q.Pluck("pt.post_id", &ids).Error
	return ids, err
}

// RelatedPostIDs — post dengan overlap term terbanyak terhadap postID (diutamakan terbaru).
func (s *TaxonomyService) RelatedPostIDs(postID string, limit int) ([]string, error) {
	var ids []string
	err := s.db.Table("post_terms a").
		Select("b.post_id").
		Joins("JOIN post_terms b ON b.term_id = a.term_id AND b.post_id <> a.post_id").
		Where("a.post_id = ?", postID).
		Group("b.post_id").
		Order("count(*) DESC, max(b.post_id) DESC").
		Limit(limit).
		Pluck("b.post_id", &ids).Error
	return ids, err
}
