package ticketsvc

import (
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
