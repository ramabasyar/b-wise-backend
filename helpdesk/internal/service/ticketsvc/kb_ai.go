package ticketsvc

import (
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/pkg/ai"
)

// ==================== KB-06: Pencarian Semantik (F4) ====================
// Embedding lokal via ollama (bge-m3, 1024 dim) di kb_articles.embedding (pgvector).
// Dev di Mac; produksi nanti ke server AI kampus via AI_BASE_URL (kontrak sama).
// Semua jalur punya fallback kata-kunci (kb.go).

var kbAI = ai.New()

// vecToLiteral — []float32 ke literal pgvector, mis. [0.1,0.2,0.3] (teks polos).
func vecToLiteral(v []float32) string {
	var b strings.Builder
	b.WriteString("[")
	for i, f := range v {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteString("]")
	return b.String()
}

// KBEmbedPending — embed artikel TERBIT yang belum punya vektor (job boot).
func (s *Service) KBEmbedPending() (int, error) {
	if !kbAI.Enabled() {
		return 0, nil
	}
	var rows []entity.KBArticle
	if err := s.db.Where("status = ? AND embedding IS NULL", "published").Limit(100).Find(&rows).Error; err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		vec, err := kbAI.Embed(r.Title + ". " + r.Body)
		if err != nil {
			continue
		}
		if err := s.db.Exec("UPDATE kb_articles SET embedding = ?::vector WHERE id = ?", vecToLiteral(vec), r.ID).Error; err == nil {
			n++
		}
	}
	return n, nil
}

// KBEmbedOne — embed ulang satu artikel (dipanggil pasca update/publish).
func (s *Service) KBEmbedOne(id string) {
	if !kbAI.Enabled() {
		return
	}
	var r entity.KBArticle
	if err := s.db.Where("id = ? AND status = ?", id, "published").First(&r).Error; err != nil {
		return
	}
	vec, err := kbAI.Embed(r.Title + ". " + r.Body)
	if err != nil {
		return
	}
	_ = s.db.Exec("UPDATE kb_articles SET embedding = ?::vector WHERE id = ?", vecToLiteral(vec), id).Error
}

// kbSemantic — urut artikel terbit menurut cosine distance ke vektor kueri.
// Return nil saat AI mati/gagal → pemanggil jatuh ke pencarian kata-kunci.
func (s *Service) kbSemantic(q, teamCode string) []entity.KBArticle {
	if !kbAI.Enabled() || strings.TrimSpace(q) == "" {
		return nil
	}
	vec, err := kbAI.Embed(q)
	if err != nil {
		return nil
	}
	sql := "SELECT * FROM kb_articles WHERE status = ? AND embedding IS NOT NULL"
	args := []interface{}{"published"}
	if teamCode != "" {
		sql += " AND team_id = (SELECT id FROM teams WHERE code = ? AND is_active LIMIT 1)"
		args = append(args, teamCode)
	}
	sql += " ORDER BY embedding <=> ?::vector LIMIT 20"
	args = append(args, vecToLiteral(vec))
	var list []entity.KBArticle
	if err := s.db.Raw(sql, args...).Scan(&list).Error; err != nil {
		return nil
	}
	return list
}
