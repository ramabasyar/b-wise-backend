package main

// waved.go — Wave D (batch 3 leftover): Jurnal.json (detail artikel jurnal unggulan)
// → posts type=jurnal-detail. Blok list ber-urutan tetap: [universitas, judul,
// penulis, terbit, kategori] supaya hook web dapat direkonstruusi deterministik.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

type jurnalDetailItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
	Cover    string `json:"cover"`
	Faculty  string `json:"faculty"`
}

type jurnalDetail struct {
	Name        string             `json:"name"`
	Universitas string             `json:"universitas"`
	Title       string             `json:"title"`
	Slug        string             `json:"slug"`
	Items       []jurnalDetailItem `json:"items"`
	Judul       string             `json:"judul"`
	Penulis     string             `json:"penulis"`
	Terbit      string             `json:"terbit"`
	Kategori    string             `json:"kategori"`
	Summary     string             `json:"summary"`
}

func jurnalDetailBlocks(e jurnalDetail) []blocks.Block {
	out := []blocks.Block{{Type: "list", Data: map[string]any{
		"items": []string{e.Universitas, e.Judul, e.Penulis, e.Terbit, e.Kategori},
		"style": "unordered",
	}}}
	out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": e.Summary}})
	if len(e.Items) > 0 {
		it := e.Items[0]
		if it.Title != "" {
			out = append(out, blocks.Block{Type: "heading", Data: map[string]any{"text": it.Title, "level": 2}})
		}
		if it.Abstract != "" {
			out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": it.Abstract}})
		}
		if u := resolveMedia(it.Cover); u != "" {
			out = append(out, blocks.Block{Type: "image", Data: map[string]any{"url": u, "alt": it.Title}})
		}
		if it.Faculty != "" {
			out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": it.Faculty}})
		}
	}
	return out
}

func importJurnalDetail(db *gorm.DB, src string) {
	fmt.Println("== Jurnal Detail (Jurnal.json) ==")
	var idList, enList []jurnalDetail
	b, err := os.ReadFile(filepath.Join(src, "Jurnal.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &idList)
	if b2, err := os.ReadFile(filepath.Join(src, "Jurnal.en.json")); err == nil {
		_ = json.Unmarshal(b2, &enList)
	}
	for i, e := range idList {
		slug := slugify(e.Slug, fmt.Sprintf("jurnal-detail-%d", i+1))
		hero := ""
		if len(e.Items) > 0 {
			hero = resolveMedia(e.Items[0].Cover)
		}
		tr := map[string]entity.PostTranslation{"id": {Title: e.Title, Author: e.Penulis, Excerpt: e.Summary, Category: e.Name, HeroAlt: e.Universitas, Blocks: jurnalDetailBlocks(e)}}
		if i < len(enList) {
			en := enList[i]
			tr["en"] = entity.PostTranslation{Title: en.Title, Author: en.Penulis, Excerpt: en.Summary, Category: en.Name, HeroAlt: en.Universitas, Blocks: jurnalDetailBlocks(en)}
		}
		upsertPostT(db, "jurnal-detail", slug, hero, tr, nil, e.Name)
	}
	fmt.Printf("  %d entri (type=jurnal-detail)\n", len(idList))
}

func importWaveD(db *gorm.DB, src string) {
	importJurnalDetail(db, src)
}
