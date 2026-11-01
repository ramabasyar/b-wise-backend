package ticketsvc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ==================== AG-13: Asisten AI (F4) ====================
// Saran SEMATA-MATA — selalu dikonfirmasi/diedit petugas (blueprint §AG-13).
// Konteks: tiket + timeline + (utk saran balasan) artikel KB teratas via semantik.

const aiAssistDisclaimer = "Saran AI — wajib dikonfirmasi dan disunting petugas sebelum dikirim."

// aiTicketContext — muat tiket + N event terakhir jadi teks utk prompt.
func (s *Service) aiTicketContext(t *entity.Ticket, nEvents int) string {
	var events []entity.TicketEvent
	s.db.Where("ticket_id = ?", t.ID).Order("created_at DESC").Limit(nEvents).Find(&events)
	var b strings.Builder
	fmt.Fprintf(&b, "Nomor: %s\nJudul: %s\nKanal: %s\nPrioritas: %s\nStatus: %s\n", t.Number, t.Subject, t.Channel, t.Priority, t.Status)
	if t.Answers != "" && t.Answers != "{}" {
		fmt.Fprintf(&b, "Isian formulir: %s\n", truncStr(t.Answers, 1500))
	}
	if len(events) > 0 {
		b.WriteString("Kronologi (terbaru dulu):\n")
		for _, e := range events {
			fmt.Fprintf(&b, "- [%s/%s] %s\n", e.Kind, e.Visibility, truncStr(e.Body, 400))
		}
	}
	return b.String()
}

func truncStr(v string, n int) string {
	if len(v) > n {
		return v[:n] + "..."
	}
	return v
}

// AISummary — ringkas tiket utk petugas (anggota tim pemilik tiket).
func (s *Service) AISummary(ticketID, userID string) (map[string]string, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	if !kbAI.Enabled() {
		return nil, fmt.Errorf("%w: fitur AI nonaktif", ErrValidation)
	}
	system := "Anda asisten helpdesk Universitas Binawan. Ringkas tiket berikut dalam 3-5 kalimat bahasa Indonesia: inti masalah, kronologi singkat, dan status terkini. Fakta saja, tanpa saran."
	out, err := kbAI.Chat(system, s.aiTicketContext(&t, 12))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return map[string]string{"summary": strings.TrimSpace(out), "disclaimer": aiAssistDisclaimer}, nil
}

// AISuggestReply — draft balasan utk pemohon; diperkaya artikel KB teratas (KB-06).
func (s *Service) AISuggestReply(ticketID, userID string) (map[string]string, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	if !kbAI.Enabled() {
		return nil, fmt.Errorf("%w: fitur AI nonaktif", ErrValidation)
	}
	kbCtx := ""
	if arts := s.kbSemantic(t.Subject, ""); len(arts) > 0 {
		top := arts[0]
		kbCtx = "Artikel KB rujukan:\n" + top.Title + "\n" + truncStr(top.Body, 1200)
	}
	system := "Anda asisten helpdesk Universitas Binawan. Susun DRAFT balasan ke pemohon tiket: bahasa Indonesia, ramah dan profesional, langsung ke inti, maksimal 3 paragraf. Gunakan artikel KB bila relevan. Jangan menjanjikan hal di luar konteks."
	prompt := s.aiTicketContext(&t, 8)
	if kbCtx != "" {
		prompt += "\n\n" + kbCtx
	}
	out, err := kbAI.Chat(system, prompt)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return map[string]string{"draft": strings.TrimSpace(out), "disclaimer": aiAssistDisclaimer}, nil
}

// ==================== OT-06: Triase AI (F4) ====================
// Saran layanan/tim/prioritas utk tiket baru (front office/email/WA).
// Kandidat = layanan TERBIT + nama tim; balasan model dipaksa JSON murni.

// AITriage - saran klasifikasi tiket baru (perm helpdesk.tickets.work, utk FO).
func (s *Service) AITriage(subject, answers string) (map[string]string, error) {
	if !kbAI.Enabled() {
		return nil, fmt.Errorf("%w: fitur AI nonaktif", ErrValidation)
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("%w: subject wajib", ErrValidation)
	}
	type svcRow struct {
		Slug     string
		Name     string
		TeamName string
	}
	var rows []svcRow
	s.db.Raw("SELECT sv.slug, sv.name, COALESCE(t.name, '') AS team_name FROM services sv LEFT JOIN teams t ON t.id = sv.team_id WHERE sv.status = ? ORDER BY sv.name LIMIT 60", "published").Scan(&rows)
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: katalog layanan kosong", ErrValidation)
	}
	var cand strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&cand, "- %s — %s (tim: %s)\n", r.Slug, r.Name, r.TeamName)
	}
	system := "Anda asisten triase helpdesk Universitas Binawan. Pilih layanan paling cocok dari daftar kandidat untuk tiket baru, tentukan prioritas, dan beri alasan singkat. Balas HANYA JSON valid tanpa teks lain, persis format: {\"layanan\": \"<nama persis dari daftar>\", \"tim\": \"<nama tim>\", \"prioritas\": \"low|normal|high|urgent\", \"alasan\": \"<1 kalimat>\"}"
	prompt := "Daftar layanan kandidat:\n" + cand.String() + "\nTiket baru:\nJudul: " + subject
	if ans := strings.TrimSpace(answers); ans != "" {
		prompt += "\nIsian formulir: " + truncStr(ans, 800)
	}
	out, err := kbAI.Chat(system, prompt)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	res := parseAIJSON(out)
	res["disclaimer"] = aiAssistDisclaimer
	return res, nil
}

// parseAIJSON - ambil objek JSON pertama dari teks model (buang pagar markdown).
func parseAIJSON(s string) map[string]string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	m := map[string]string{}
	if start < 0 || end <= start {
		m["raw"] = truncStr(s, 400)
		return m
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(s[start:end+1]), &parsed); err != nil {
		m["raw"] = truncStr(s, 400)
		return m
	}
	for _, k := range []string{"layanan", "tim", "prioritas", "alasan"} {
		if v, ok := parsed[k].(string); ok {
			m[k] = v
		}
	}
	return m
}
