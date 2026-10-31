package main

// waveb.go — Wave B (batch 3): profil pimpinan → posts per-person.
// direktorat/rektorat/founder → type direktorat/rektorat/pendiri;
// pimpinanFakultas → type pimpinan-fakultas dengan Category = nama fakultas.
// Blocks: paragraph(deskripsi) + image(detailSvg, alt="detail").

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
)

type profilePerson struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Image       string `json:"image"`
	DetailSvg   string `json:"detailSvg"`
	Description string `json:"description"`
}

type profilePage struct {
	TitleMain   string          `json:"titleMain"`
	TitleSub    string          `json:"titleSub"`
	Description string          `json:"description"`
	BoxItems    []profilePerson `json:"boxItems"`
}

type fkkFaculty struct {
	Name   string          `json:"name"`
	Photos []profilePerson `json:"photos"`
}

type fkkPage struct {
	TitleMain   string       `json:"titleMain"`
	TitleSub    string       `json:"titleSub"`
	Description string       `json:"description"`
	Faculties   []fkkFaculty `json:"faculties"`
}

func personBlocks(p profilePerson) []blocks.Block {
	out := []blocks.Block{}
	if p.Description != "" {
		out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": p.Description}})
	}
	if u := resolveMedia(p.DetailSvg); u != "" {
		out = append(out, blocks.Block{Type: "image", Data: map[string]any{"url": u, "alt": "detail"}})
	}
	return out
}

func personTr(p profilePerson, cat string) entity.PostTranslation {
	return entity.PostTranslation{Title: p.Name, Excerpt: p.Title, Category: cat, Blocks: personBlocks(p)}
}

func importProfile(db *gorm.DB, src, file, typ string) {
	fmt.Println("== Profil:", typ, "==")
	var idPage, enPage profilePage
	b, err := os.ReadFile(filepath.Join(src, file+".json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &idPage)
	if b2, err := os.ReadFile(filepath.Join(src, file+".en.json")); err == nil {
		_ = json.Unmarshal(b2, &enPage)
	}
	used := map[string]bool{}
	for i, it := range idPage.BoxItems {
		slug := slugify(typ+" "+it.Name, fmt.Sprintf("%s-%d", typ, i+1))
		if used[slug] {
			slug = fmt.Sprintf("%s-%d", slug, i+1)
		}
		used[slug] = true
		tr := map[string]entity.PostTranslation{"id": personTr(it, "Profil")}
		if i < len(enPage.BoxItems) {
			tr["en"] = personTr(enPage.BoxItems[i], "Profile")
		}
		upsertPostT(db, typ, slug, resolveMedia(it.Image), tr, nil, "Profil")
	}
	fmt.Printf("  %d orang (type=%s)\n", len(idPage.BoxItems), typ)
}

func importPimpinanFakultas(db *gorm.DB, src string) {
	fmt.Println("== Pimpinan Fakultas ==")
	var idPage, enPage fkkPage
	b, err := os.ReadFile(filepath.Join(src, "pimpinanFakultas.json"))
	if err != nil {
		fmt.Println("  skip:", err)
		return
	}
	_ = json.Unmarshal(b, &idPage)
	if b2, err := os.ReadFile(filepath.Join(src, "pimpinanFakultas.en.json")); err == nil {
		_ = json.Unmarshal(b2, &enPage)
	}
	n := 0
	used := map[string]bool{}
	for fi, f := range idPage.Faculties {
		enFaculty := ""
		if fi < len(enPage.Faculties) {
			enFaculty = enPage.Faculties[fi].Name
		}
		for i, it := range f.Photos {
			n++
			slug := slugify("pimpinan "+it.Name, fmt.Sprintf("pimpinan-f%d-%d", fi+1, i+1))
			if used[slug] {
				slug = fmt.Sprintf("%s-%d", slug, i+1)
			}
			used[slug] = true
			tr := map[string]entity.PostTranslation{"id": personTr(it, f.Name)}
			if fi < len(enPage.Faculties) && i < len(enPage.Faculties[fi].Photos) {
				tr["en"] = personTr(enPage.Faculties[fi].Photos[i], enFaculty)
			}
			upsertPostT(db, "pimpinan-fakultas", slug, resolveMedia(it.Image), tr, nil, f.Name)
		}
	}
	fmt.Printf("  %d orang, %d fakultas (type=pimpinan-fakultas)\n", n, len(idPage.Faculties))
}

func importWaveB(db *gorm.DB, src string) {
	importProfile(db, src, "direktorat", "direktorat")
	importProfile(db, src, "rektorat", "rektorat")
	importProfile(db, src, "founder", "pendiri")
	importPimpinanFakultas(db, src)
}
