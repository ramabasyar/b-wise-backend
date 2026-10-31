package main

// wavec.go — Wave C (batch 3): keluarga jurnal → CMS.
// jurnalListingData (id+en, 3 jurnal × item sitasi) → posts type=jurnal-artikel
//   (Category = judul jurnal per-locale → grouping web).
// artikelData (id+en, 34 artikel penuh) → posts type=artikel dengan blocks
//   terstruktur (cta buttonLink + paragraphs + sections: heading/paragraf/gambar).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

type jurnalArticle struct {
	ID               any    `json:"id"`
	Title            string `json:"title"`
	Author           string `json:"author"`
	PublishedDate    string `json:"publishedDate"`
	Abstract         string `json:"abstract"`
	Image            string `json:"image"`
	ButtonLink       string `json:"buttonLink"`
	ContentStructure *struct {
		Paragraphs []string `json:"paragraphs"`
		Sections   []struct {
			Heading    string   `json:"heading"`
			Images     []string `json:"images"`
			Paragraphs []string `json:"paragraphs"`
		} `json:"sections"`
	} `json:"contentStructure"`
}

type jurnalSection struct {
	ID           string          `json:"id"`
	Faculty      string          `json:"faculty"`
	FullName     string          `json:"fullName"`
	JournalTitle string          `json:"journalTitle"`
	University   string          `json:"University"`
	Description  string          `json:"description"`
	Color        string          `json:"color"`
	Items        []jurnalArticle `json:"items"`
}

func articleBlocks(a jurnalArticle) []blocks.Block {
	out := []blocks.Block{}
	if a.ButtonLink != "" {
		out = append(out, blocks.Block{Type: "cta_button", Data: map[string]any{"label": "Baca", "url": a.ButtonLink}})
	}
	cs := a.ContentStructure
	if cs == nil {
		return out
	}
	for _, p := range cs.Paragraphs {
		if p != "" {
			out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": p}})
		}
	}
	for _, s := range cs.Sections {
		if s.Heading != "" {
			out = append(out, blocks.Block{Type: "heading", Data: map[string]any{"text": s.Heading, "level": 2}})
		}
		for _, p := range s.Paragraphs {
			if p != "" {
				out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": p}})
			}
		}
		for _, im := range s.Images {
			if u := resolveMedia(im); u != "" {
				out = append(out, blocks.Block{Type: "image", Data: map[string]any{"url": u, "alt": a.Title}})
			}
		}
	}
	return out
}

func parseTanggalISO(s string) *time.Time {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	return nil
}

func importJurnalArticles(db *gorm.DB, src string) {
	fmt.Println("== Jurnal Listing (jurnalListingData) ==")
	var idList, enList []jurnalSection
	b, err := os.ReadFile(filepath.Join(src, "jurnalListingData.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &idList)
	if b2, err := os.ReadFile(filepath.Join(src, "jurnalListingData.en.json")); err == nil {
		_ = json.Unmarshal(b2, &enList)
	}
	n := 0
	used := map[string]bool{}
	for si, sec := range idList {
		var enSec *jurnalSection
		if si < len(enList) {
			enSec = &enList[si]
		}
		for i, it := range sec.Items {
			n++
			slug := slugify("jurnal-"+it.Title, fmt.Sprintf("jurnal-%s-%d", sec.ID, i+1))
			if used[slug] {
				slug = fmt.Sprintf("%s-%d", slug, i+1)
			}
			used[slug] = true
			tr := map[string]entity.PostTranslation{"id": {Title: it.Title, Author: it.Author, Excerpt: it.Abstract, Category: sec.JournalTitle, Blocks: articleBlocks(it)}}
			if enSec != nil && i < len(enSec.Items) {
				e := enSec.Items[i]
				tr["en"] = entity.PostTranslation{Title: e.Title, Author: e.Author, Excerpt: e.Abstract, Category: enSec.JournalTitle, Blocks: articleBlocks(e)}
			}
			upsertPostT(db, "jurnal-artikel", slug, resolveMedia(it.Image), tr, parseTanggalISO(it.PublishedDate), sec.JournalTitle)
		}
	}
	fmt.Printf("  %d artikel sitasi (type=jurnal-artikel, %d jurnal)\n", n, len(idList))
}

func importArtikel(db *gorm.DB, src string) {
	fmt.Println("== Artikel (artikelData) ==")
	var idList, enList []jurnalSection
	b, err := os.ReadFile(filepath.Join(src, "artikelData.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &idList)
	if b2, err := os.ReadFile(filepath.Join(src, "artikelData.en.json")); err == nil {
		_ = json.Unmarshal(b2, &enList)
	}
	n := 0
	used := map[string]bool{}
	for si, sec := range idList {
		var enSec *jurnalSection
		if si < len(enList) {
			enSec = &enList[si]
		}
		for i, it := range sec.Items {
			n++
			slug := slugify("artikel-"+it.Title, fmt.Sprintf("artikel-%d", i+1))
			if used[slug] {
				slug = fmt.Sprintf("%s-%d", slug, i+1)
			}
			used[slug] = true
			tr := map[string]entity.PostTranslation{"id": {Title: it.Title, Author: it.Author, Excerpt: it.Abstract, Category: "Artikel", Blocks: articleBlocks(it)}}
			if enSec != nil && i < len(enSec.Items) {
				e := enSec.Items[i]
				tr["en"] = entity.PostTranslation{Title: e.Title, Author: e.Author, Excerpt: e.Abstract, Category: "Articles", Blocks: articleBlocks(e)}
			}
			upsertPostT(db, "artikel", slug, resolveMedia(it.Image), tr, parseTanggalISO(it.PublishedDate), "Artikel")
		}
	}
	fmt.Printf("  %d artikel penuh (type=artikel)\n", n)
}

func importWaveC(db *gorm.DB, src string) {
	importJurnalArticles(db, src)
	importArtikel(db, src)
}
