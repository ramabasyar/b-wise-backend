// Package blocks — content blocks portable utk tubuh konten (posts/pages).
// Tiap tipe block punya validator; data disimpan jsonb per-locale di translations.
package blocks

import (
	"fmt"
	"strings"
)

// Block — satu unit konten. Data longgar (jsonb), divalidasi per tipe.
type Block struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// Tipe block terdaftar (F0). Tipe baru didaftarkan di validators.
var validators = map[string]func(map[string]any) error{
	"heading":      vHeading,
	"paragraph":    vParagraph,
	"image":        vImage,
	"gallery":      vGallery,
	"quote":        vQuote,
	"video_embed":  vVideoEmbed,
	"table":        vTable,
	"cta_button":   vCTAButton,
}

// ValidTypes — daftar tipe block yang diterima (utk dokumentasi/editor UI).
func ValidTypes() []string {
	ts := make([]string, 0, len(validators))
	for t := range validators {
		ts = append(ts, t)
	}
	return ts
}

// Validate — pastikan setiap block bertipe dikenal + field wajib terisi.
func Validate(blocks []Block) error {
	for i, b := range blocks {
		v, ok := validators[b.Type]
		if !ok {
			return fmt.Errorf("block #%d: tipe %q tidak dikenal (valid: heading, paragraph, image, gallery, quote, video_embed, table, cta_button)", i, b.Type)
		}
		if b.Data == nil {
			return fmt.Errorf("block #%d (%s): data kosong", i, b.Type)
		}
		if err := v(b.Data); err != nil {
			return fmt.Errorf("block #%d (%s): %w", i, b.Type, err)
		}
	}
	return nil
}

// ---------- helpers ----------

func reqStr(d map[string]any, key string, maxLen int) (string, error) {
	v, ok := d[key]
	if !ok {
		return "", fmt.Errorf("field %q wajib ada", key)
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("field %q harus string non-kosong", key)
	}
	if maxLen > 0 && len(s) > maxLen {
		return "", fmt.Errorf("field %q maksimal %d karakter", key, maxLen)
	}
	return s, nil
}

func optStr(d map[string]any, key string, maxLen int) (string, error) {
	v, ok := d[key]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("field %q harus string", key)
	}
	if maxLen > 0 && len(s) > maxLen {
		return "", fmt.Errorf("field %q maksimal %d karakter", key, maxLen)
	}
	return s, nil
}

func reqStrList(d map[string]any, key string) ([]string, error) {
	v, ok := d[key]
	if !ok {
		return nil, fmt.Errorf("field %q wajib ada", key)
	}
	raw, ok := v.([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("field %q harus array non-kosong", key)
	}
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		s, ok := x.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("field %q harus array of string non-kosong", key)
		}
		out = append(out, s)
	}
	return out, nil
}

// ---------- validators per tipe ----------

func vHeading(d map[string]any) error {
	if _, err := reqStr(d, "text", 300); err != nil {
		return err
	}
	if lvl, ok := d["level"]; ok && lvl != nil {
		f, ok := lvl.(float64)
		if !ok || f < 1 || f > 4 {
			return fmt.Errorf("level harus integer 1-4")
		}
	}
	return nil
}

func vParagraph(d map[string]any) error {
	_, err := reqStr(d, "text", 50000)
	return err
}

func vImage(d map[string]any) error {
	url, uerr := optStr(d, "url", 1000)
	media, merr := optStr(d, "media_id", 36)
	if uerr != nil || merr != nil {
		return fmt.Errorf("url/media_id tidak valid")
	}
	if url == "" && media == "" {
		return fmt.Errorf("salah satu dari url / media_id wajib ada")
	}
	if _, err := optStr(d, "alt", 500); err != nil {
		return err
	}
	return nil
}

func vGallery(d map[string]any) error {
	if _, err := reqStrList(d, "media_ids"); err != nil {
		return err
	}
	_, err := optStr(d, "caption", 500)
	return err
}

func vQuote(d map[string]any) error {
	if _, err := reqStr(d, "text", 2000); err != nil {
		return err
	}
	_, err := optStr(d, "author", 200)
	return err
}

func vVideoEmbed(d map[string]any) error {
	u, err := reqStr(d, "url", 1000)
	if err != nil {
		return err
	}
	if !strings.Contains(u, "youtube.com") && !strings.Contains(u, "youtu.be") &&
		!strings.Contains(u, "vimeo.com") && !strings.Contains(u, "binawan.ac.id") {
		return fmt.Errorf("hanya youtube/vimeo/self-hosted yang diizinkan")
	}
	return nil
}

func vTable(d map[string]any) error {
	if _, err := reqStrList(d, "headers"); err != nil {
		return err
	}
	raw, ok := d["rows"]
	if !ok {
		return fmt.Errorf("field %q wajib ada", "rows")
	}
	rows, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("field %q harus array of array", "rows")
	}
	for i, r := range rows {
		if _, ok := r.([]any); !ok {
			return fmt.Errorf("rows[%d] harus array", i)
		}
	}
	return nil
}

func vCTAButton(d map[string]any) error {
	if _, err := reqStr(d, "text", 100); err != nil {
		return err
	}
	u, err := reqStr(d, "url", 1000)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "/") {
		return fmt.Errorf("url harus http(s) atau path relatif")
	}
	return nil
}
