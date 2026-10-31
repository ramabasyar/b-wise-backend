package main

// wavea.go — impor koleksi lanjutan (batch 3 wave A): Pengabdian (+en),
// BeasiswaPost, Kalender, MediaData(galeri) → posts ber-type. Idempotent by slug.
// Referensi gambar diselesaikan lewat legacy-media-map (URL MinIO).

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

var mediaMap = map[string]map[string]any{}

func loadMediaMap() {
	p := "/Users/macbook/project/react/binawan/src/data/legacy-media-map.json"
	b, err := os.ReadFile(p)
	if err != nil {
		fmt.Println("  [media-map] tidak termuat:", err)
		return
	}
	_ = json.Unmarshal(b, &mediaMap)
	fmt.Printf("  [media-map] %d kunci\n", len(mediaMap))
}

func resolveMedia(ref string) string {
	if ref == "" || strings.HasPrefix(ref, "http") {
		return ref
	}
	n := strings.TrimPrefix(ref, "src/")
	n = strings.TrimPrefix(n, "assets/")
	n = strings.TrimPrefix(n, "Images/")
	n = strings.TrimPrefix(n, "/")
	if e, ok := mediaMap[n]; ok {
		if u, _ := e["url"].(string); u != "" {
			return u
		}
	}
	base := path.Base(n)
	for k, e := range mediaMap {
		if k == base || strings.HasSuffix(k, "/"+base) {
			if u, _ := e["url"].(string); u != "" {
				return u
			}
		}
	}
	return ref
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string, fallback string) string {
	out := slugRe.ReplaceAllString(strings.ToLower(s), "-")
	out = strings.Trim(out, "-")
	if len(out) > 90 {
		out = out[:90]
	}
	if out == "" {
		return fallback
	}
	return out
}

var bulanID = map[string]time.Month{"januari": time.January, "februari": time.February, "maret": time.March, "april": time.April, "mei": time.May, "juni": time.June, "juli": time.July, "agustus": time.August, "september": time.September, "oktober": time.October, "november": time.November, "desember": time.December}

func parseTanggalID(s string) *time.Time {
	re := regexp.MustCompile(`(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})`)
	m := re.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	d, _ := strconv.Atoi(m[1])
	y, _ := strconv.Atoi(m[3])
	mo, ok := bulanID[strings.ToLower(m[2])]
	if !ok {
		return nil
	}
	t := time.Date(y, mo, d, 8, 0, 0, 0, time.Local)
	return &t
}

func upsertPostT(db *gorm.DB, typ, slug, heroURL string, tr map[string]entity.PostTranslation, when *time.Time, cat string) {
	var p entity.Post
	err := db.First(&p, "slug = ?", slug).Error
	if err != nil {
		p = entity.Post{Slug: slug, Type: typ, Status: entity.StatusPublished, Source: "import"}
	}
	p.Type = typ
	p.Status = entity.StatusPublished
	p.Source = "import"
	p.HeroURL = heroURL
	p.PublishAt = when
	p.Translations = "{}"
	for loc, t := range tr {
		if t.Category == "" {
			t.Category = cat
		}
		if t.Author == "" {
			t.Author = "Humas Universitas Binawan"
		}
		p.SetTr(loc, t)
	}
	if err != nil {
		if e := db.Create(&p).Error; e != nil {
			fmt.Println("  CREATE GAGAL", slug, e)
		}
	} else {
		if e := db.Save(&p).Error; e != nil {
			fmt.Println("  UPDATE GAGAL", slug, e)
		}
	}
}

func parasBlocks(text string) []blocks.Block {
	out := []blocks.Block{}
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if len(para) > 40000 {
			para = para[:40000]
		}
		out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": para}})
	}
	return out
}

func imageBlocks(refs []string, alt string) []blocks.Block {
	out := []blocks.Block{}
	for _, r := range refs {
		u := resolveMedia(r)
		if u == "" {
			continue
		}
		out = append(out, blocks.Block{Type: "image", Data: map[string]any{"url": u, "alt": alt}})
	}
	return out
}

func rendererBlocksToBwm(content []any) []blocks.Block {
	out := []blocks.Block{}
	for _, c := range content {
		m, ok := c.(map[string]any)
		if !ok {
			if s, ok := c.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": s}})
			}
			continue
		}
		switch m["type"] {
		case "paragraph":
			if txt, _ := m["text"].(string); txt != "" {
				out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": txt}})
			}
		case "image":
			src, _ := m["src"].(string)
			alt, _ := m["alt"].(string)
			if u := resolveMedia(src); u != "" {
				out = append(out, blocks.Block{Type: "image", Data: map[string]any{"url": u, "alt": alt}})
			}
		case "heading":
			txt, _ := m["text"].(string)
			out = append(out, blocks.Block{Type: "heading", Data: map[string]any{"text": txt, "level": 2}})
		case "list":
			if raw, ok := m["items"].([]any); ok && len(raw) > 0 {
				out = append(out, blocks.Block{Type: "list", Data: map[string]any{"items": raw, "style": "unordered"}})
			}
		}
	}
	return out
}

type pengabdianItem struct {
	ID      string   `json:"id"`
	Slug    string   `json:"slug"`
	Title   string   `json:"title"`
	Date    string   `json:"date"`
	Excerpt string   `json:"excerpt"`
	Image   string   `json:"image"`
	Images  []string `json:"images"`
	Content string   `json:"content"`
}

func importPengabdian(db *gorm.DB, src string) {
	fmt.Println("== Pengabdian ==")
	var idList, enList []pengabdianItem
	b, err := os.ReadFile(filepath.Join(src, "Pengabdian.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &idList)
	if b2, err := os.ReadFile(filepath.Join(src, "Pengabdian.en.json")); err == nil {
		_ = json.Unmarshal(b2, &enList)
	}
	used := map[string]bool{}
	for i, it := range idList {
		slug := slugify(it.Title, "pengabdian-"+it.ID)
		if used[slug] {
			slug = slug + "-" + it.ID
		}
		used[slug] = true
		when := parseTanggalID(it.Date)
		tr := map[string]entity.PostTranslation{"id": {Title: it.Title, Excerpt: it.Excerpt, Category: "Pengabdian", Blocks: append(parasBlocks(it.Content), imageBlocks(it.Images, it.Title)...)}}
		if i < len(enList) {
			e := enList[i]
			tr["en"] = entity.PostTranslation{Title: e.Title, Excerpt: e.Excerpt, Category: "Community Service", Blocks: append(parasBlocks(e.Content), imageBlocks(e.Images, e.Title)...)}
		}
		upsertPostT(db, "pengabdian", slug, resolveMedia(it.Image), tr, when, "Pengabdian")
	}
	fmt.Printf("  %d item (type=pengabdian)\n", len(idList))
}

type beasiswaItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Date      string `json:"date"`
	Thumbnail string `json:"thumbnail"`
	Content   []any  `json:"content"`
}

func importBeasiswa(db *gorm.DB, src string) {
	fmt.Println("== BeasiswaPost ==")
	var list []beasiswaItem
	b, err := os.ReadFile(filepath.Join(src, "BeasiswaPost.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &list)
	for _, it := range list {
		slug := slugify("beasiswa "+it.Title, "beasiswa-"+it.ID)
		var when *time.Time
		if t2, err := time.Parse("2006-01-02", it.Date); err == nil {
			when = &t2
		}
		tr := map[string]entity.PostTranslation{"id": {Title: it.Title, Author: it.Author, Category: "Beasiswa", Blocks: rendererBlocksToBwm(it.Content)}}
		upsertPostT(db, "beasiswa", slug, resolveMedia(it.Thumbnail), tr, when, "Beasiswa")
	}
	fmt.Printf("  %d item (type=beasiswa)\n", len(list))
}

type kalenderItem struct {
	ID    int    `json:"id"`
	Image string `json:"image"`
}

func importKalender(db *gorm.DB, src string) {
	fmt.Println("== Kalender ==")
	var list []kalenderItem
	b, err := os.ReadFile(filepath.Join(src, "Kalender.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &list)
	for _, it := range list {
		slug := fmt.Sprintf("kalender-akademik-%d", it.ID)
		u := resolveMedia(it.Image)
		tr := map[string]entity.PostTranslation{"id": {Title: fmt.Sprintf("Kalender Akademik %d", it.ID), Category: "Kalender", Blocks: []blocks.Block{{Type: "image", Data: map[string]any{"url": u, "alt": "Kalender Akademik"}}}}}
		upsertPostT(db, "kalender", slug, u, tr, nil, "Kalender")
	}
	fmt.Printf("  %d item (type=kalender)\n", len(list))
}

type galeriItem struct {
	ID          string `json:"id"`
	Image       string `json:"image"`
	Title       string `json:"title"`
	Date        string `json:"date"`
	Description string `json:"description"`
	ExternalURL string `json:"externalUrl"`
}

func importGaleri(db *gorm.DB, src string) {
	fmt.Println("== MediaData (galeri) ==")
	var list []galeriItem
	b, err := os.ReadFile(filepath.Join(src, "MediaData.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &list)
	for _, it := range list {
		slug := slugify("galeri "+it.Title, "galeri-"+it.ID)
		body := it.Description
		if it.ExternalURL != "" {
			body += "\n\nTautan eksternal: " + it.ExternalURL
		}
		tr := map[string]entity.PostTranslation{"id": {Title: it.Title, Excerpt: it.Description, Category: "Galeri", Blocks: parasBlocks(body)}}
		upsertPostT(db, "galeri", slug, resolveMedia(it.Image), tr, nil, "Galeri")
	}
	fmt.Printf("  %d item (type=galeri)\n", len(list))
}

func importWaveA(db *gorm.DB, src string) {
	loadMediaMap()
	importPengabdian(db, src)
	importBeasiswa(db, src)
	importKalender(db, src)
	importGaleri(db, src)
}
