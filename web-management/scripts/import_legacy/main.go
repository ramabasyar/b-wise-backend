// import_legacy — import JSON lama web binawan → DB BWM (satu kali, F0).
// Sumber: KabarKampus(+.en), KabarTerbaru, Slider, EventKampus.
// Mode: -apply (default, upsert idempotent by slug) | -check (parity saja).
// Jalankan dari root service: go run ./scripts/import_legacy -apply
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/rama/b-wise/web-management/internal/domain/blocks"
	"github.com/rama/b-wise/web-management/internal/domain/entity"
	"github.com/rama/b-wise/web-management/internal/domain/service"
)

func main() {
	src := flag.String("src", "/Users/macbook/project/react/binawan/src/assets/data", "dir data JSON lama")
	check := flag.Bool("check", false, "hanya verifikasi parity (tanpa tulis)")
	flag.Parse()

	db := mustDB()
	if *check {
		os.Exit(runCheck(db, *src))
	}
	runImport(db, *src)
	fmt.Println("\n=== PARITY PASCA-IMPORT ===")
	os.Exit(runCheck(db, *src))
}

// ==================== DB ====================

func mustDB() *gorm.DB {
	ev := map[string]string{}
	if b, err := os.ReadFile(".env"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if i := strings.Index(l, "="); i > 0 && !strings.HasPrefix(l, "#") {
				ev[strings.TrimSpace(l[:i])] = strings.TrimSpace(l[i+1:])
			}
		}
	}
	get := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		if v := ev[k]; v != "" {
			return v
		}
		return d
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Jakarta",
		get("DB_HOST", "localhost"), get("DB_PORT", "5432"), get("DB_USER", "ssouser"),
		get("DB_PASSWORD", ""), get("DB_NAME", "bwm_db"))
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		fmt.Println("DB GAGAL:", err)
		os.Exit(2)
	}
	return db
}

// ==================== helpers ====================

func readJSON(path string) []map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("SKIP (tidak ada):", path)
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		fmt.Println("GAGAL PARSE:", path, err)
		os.Exit(2)
	}
	return rows
}

var idMonths = map[string]time.Month{
	"Januari": time.January, "Februari": time.February, "Maret": time.March, "April": time.April,
	"Mei": time.May, "Juni": time.June, "Juli": time.July, "Agustus": time.August,
	"September": time.September, "Oktober": time.October, "November": time.November, "Desember": time.December,
}

// parseDateID — "26 Agustus 2026", "15 Desember 2025 - 09.00 WIB", "2026-09-25".
func parseDateID(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	if i := strings.Index(s, " - "); i > 0 { // buang "- 09.00 WIB"
		s = s[:i]
	}
	parts := strings.Fields(s)
	if len(parts) < 3 {
		return nil
	}
	d, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil
	}
	m, ok := idMonths[parts[1]]
	if !ok {
		if mm, err2 := time.Parse("January", parts[1]); err2 == nil {
			m = mm.Month()
		} else {
			return nil
		}
	}
	y, err := strconv.Atoi(parts[2])
	if err != nil {
		return nil
	}
	t := time.Date(y, m, d, 8, 0, 0, 0, time.Local) // default 08:00 pagi
	return &t
}

func str(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func paragraphsOf(m map[string]any) []string {
	cs, ok := m["contentStructure"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := cs["paragraphs"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		if s, ok := p.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func upsertPost(db *gorm.DB, slug, heroURL string, tr map[string]entity.PostTranslation, when *time.Time, cat string) {
	var p entity.Post
	err := db.First(&p, "slug = ?", slug).Error
	if err != nil {
		p = entity.Post{Slug: slug, Type: "news", Status: entity.StatusPublished, Source: "import"}
	}
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

func ensureBlocks(paras []string, extraImages []string) []blocks.Block {
	var out []blocks.Block
	for _, t := range paras {
		out = append(out, blocks.Block{Type: "paragraph", Data: map[string]any{"text": t}})
	}
	for _, img := range extraImages {
		if img != "" {
			out = append(out, blocks.Block{Type: "image", Data: map[string]any{"url": img, "alt": ""}})
		}
	}
	_ = blocks.Validate(out)
	return out
}

// ==================== import ====================

func runImport(db *gorm.DB, src string) {
	// --- KabarKampus (+ .en) → Post ---
	rows := readJSON(src + "/KabarKampus.json")
	en := readJSON(src + "/KabarKampus.en.json")
	enBySlug := map[string]map[string]any{}
	for _, r := range en {
		if s := service.Slugify(str(r, "slug")); s != "" {
			enBySlug[s] = r
		}
	}
	fmt.Printf("KabarKampus: %d baris (en: %d)\n", len(rows), len(en))
	for _, r := range rows {
		slug := service.Slugify(str(r, "slug"))
		if slug == "" {
			slug = service.Slugify(str(r, "title"))
		}
		tr := map[string]entity.PostTranslation{
			"id": {
				Title: str(r, "title"), Excerpt: str(r, "excerpt"), Author: str(r, "author"),
				Category: str(r, "category"),
				Blocks:   ensureBlocks(paragraphsOf(r), []string{str(r, "image2"), str(r, "image3")}),
			},
		}
		if er, ok := enBySlug[slug]; ok {
			tr["en"] = entity.PostTranslation{
				Title: str(er, "title"), Excerpt: str(er, "excerpt"), Author: str(er, "author"),
				Category: str(er, "category"),
				Blocks:   ensureBlocks(paragraphsOf(er), []string{str(er, "image2"), str(er, "image3")}),
			}
		}
		upsertPost(db, slug, str(r, "image"), tr, parseDateID(str(r, "date")), "")
	}
	fmt.Println("  → posts (kabar kampus) OK")

	// --- KabarTerbaru → Post ---
	rows = readJSON(src + "/KabarTerbaru.json")
	fmt.Printf("KabarTerbaru: %d baris\n", len(rows))
	for _, r := range rows {
		slug := service.Slugify(str(r, "title"))
		var paras []string
		if fd := str(r, "fullDescription"); fd != "" {
			paras = []string{fd}
		}
		tr := map[string]entity.PostTranslation{
			"id": {
				Title: str(r, "title"), Excerpt: str(r, "excerpt"), Category: str(r, "category"),
				Author: "Humas Universitas Binawan",
				Blocks: ensureBlocks(paras, nil),
			},
		}
		upsertPost(db, slug, str(r, "image"), tr, parseDateID(str(r, "date")), str(r, "category"))
	}
	fmt.Println("  → posts (kabar terbaru) OK")

	// --- Slider → Banner ---
	rows = readJSON(src + "/Slider.json")
	fmt.Printf("Slider: %d baris\n", len(rows))
	for _, r := range rows {
		b := entity.Banner{
			ImageURL: str(r, "image"), Active: true,
			CreatedBy: "import", UpdatedBy: "import",
		}
		if v, ok := r["id"]; ok {
			if f, ok := v.(float64); ok {
				b.SortOrder = int(f)
			}
		}
		b.SetTr("id", entity.BannerTranslation{Title: str(r, "title"), Description: str(r, "description")})
		var exist entity.Banner
		if e := db.First(&exist, "image_url = ? AND sort_order = ?", b.ImageURL, b.SortOrder).Error; e != nil {
			db.Create(&b)
		} else {
			exist.ImageURL = b.ImageURL
			exist.SortOrder = b.SortOrder
			exist.Active = true
			exist.UpdatedBy = "import"
			exist.Translations = "{}"
			exist.SetTr("id", entity.BannerTranslation{Title: str(r, "title"), Description: str(r, "description")})
			db.Save(&exist)
		}
	}
	fmt.Println("  → banners OK")

	// --- EventKampus → Event ---
	rows = readJSON(src + "/EventKampus.json")
	fmt.Printf("EventKampus: %d baris\n", len(rows))
	for _, r := range rows {
		slug := service.Slugify(str(r, "title"))
		e := entity.Event{
			Slug: slug, Status: entity.StatusPublished, Location: str(r, "location"),
			HeroURL: str(r, "image"), Month: str(r, "month"),
			PublishAt: parseDateID(str(r, "date")), CreatedBy: "import", UpdatedBy: "import",
		}
		e.EventDate = parseDateID(str(r, "date"))
		if v, ok := r["year"]; ok {
			if f, ok := v.(float64); ok {
				e.Year = int(f)
			}
		}
		e.SetTr("id", entity.EventTranslation{
			Title: str(r, "title"), Subtitle: str(r, "subtitle"), Description: str(r, "description"),
		})
		var exist entity.Event
		if x := db.First(&exist, "slug = ?", slug).Error; x != nil {
			db.Create(&e)
		} else {
			e.ID = exist.ID
			db.Save(&e)
		}
	}
	fmt.Println("  → events OK")
}

// ==================== parity ====================

func runCheck(db *gorm.DB, src string) int {
	ok := true
	fail := func(f string, a ...any) {
		fmt.Printf("  FAIL: "+f+"\n", a...)
		ok = false
	}

	kk := readJSON(src + "/KabarKampus.json")
	kt := readJSON(src + "/KabarTerbaru.json")
	sl := readJSON(src + "/Slider.json")
	ev := readJSON(src + "/EventKampus.json")
	en := readJSON(src + "/KabarKampus.en.json")

	var posts []entity.Post
	db.Find(&posts, "source = ?", "import")
	bySlug := map[string]entity.Post{}
	for _, p := range posts {
		bySlug[p.Slug] = p
	}

	// 1) jumlah
	if len(posts) != len(kk)+len(kt) {
		fail("posts import = %d, sumber = %d", len(posts), len(kk)+len(kt))
	} else {
		fmt.Printf("  PASS posts jumlah: %d\n", len(posts))
	}

	// 2) setiap KabarKampus: slug ada + title id cocok
	for _, r := range kk {
		slug := service.Slugify(str(r, "slug"))
		p, ok2 := bySlug[slug]
		if !ok2 {
			fail("kabar kampus slug %q tidak ada di DB", slug)
			continue
		}
		tr, _ := entity.TrResolve(entity.TrOf[entity.PostTranslation](p.Translations), "id")
		if tr.Title != str(r, "title") {
			fail("title beda utk %q", slug)
		}
	}
	fmt.Println("  PASS kabar kampus: slug+title (kecuali FAIL di atas)")

	// 3) EN coverage
	enCount := 0
	for _, r := range kk {
		if p, ok2 := bySlug[service.Slugify(str(r, "slug"))]; ok2 {
			tr := entity.TrOf[entity.PostTranslation](p.Translations)
			if t, ok3 := tr["en"]; ok3 && t.Title != "" {
				enCount++
			}
		}
	}
	if enCount != len(en) {
		fail("EN posts = %d, sumber en = %d", enCount, len(en))
	} else {
		fmt.Printf("  PASS EN coverage: %d/%d\n", enCount, len(en))
	}

	// 4) banners
	var banners []entity.Banner
	db.Find(&banners)
	if len(banners) != len(sl) {
		fail("banners = %d, slider = %d", len(banners), len(sl))
	} else {
		fmt.Printf("  PASS banners jumlah: %d\n", len(banners))
	}

	// 5) events (sumber punya judul ganda — bandingkan dgn slug UNIK)
	var events []entity.Event
	db.Find(&events)
	evUnique := map[string]bool{}
	for _, r := range ev {
		evUnique[service.Slugify(str(r, "title"))] = true
	}
	if len(events) != len(evUnique) {
		fail("events = %d, sumber (slug unik) = %d", len(events), len(evUnique))
	} else {
		fmt.Printf("  PASS events jumlah: %d (slug unik dari %d baris)\n", len(events), len(ev))
	}
	for _, r := range ev {
		slug := service.Slugify(str(r, "title"))
		var e entity.Event
		if x := db.First(&e, "slug = ?", slug).Error; x != nil {
			fail("event %q tidak ada", slug)
		}
	}

	if ok {
		fmt.Println("PARITY: ALL PASS")
		return 0
	}
	fmt.Println("PARITY: HAS FAIL")
	return 1
}
