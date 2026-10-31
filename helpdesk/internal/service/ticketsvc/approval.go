package ticketsvc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

// FlowStep - satu langkah approval pada konfigurasi layanan (jsonb approval_flow).
// Rule: person (user_id eksplisit) | direct_supervisor (resolusi HC-3) |
// parallel (users: beberapa approver sekaligus; mode all = semua setuju, any = cukup satu - AP-05).
// DueMinutes: batas keputusan dalam menit jam layanan (AP-04; default 480).
type FlowStep struct {
	Step       int      `json:"step"`
	Rule       string   `json:"rule"`
	UserID     string   `json:"user_id,omitempty"`
	Users      []string `json:"users,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	DueMinutes int      `json:"due_minutes,omitempty"`
}

// flowHasSteps - true bila flow JSON berisi minimal satu langkah valid.
// Flow kosong ('{}'/'[]'/invalid) TIDAK boleh memasukkan tiket ke menunggu_approval
// karena tidak ada approval yang bisa diputuskan (jalan buntu - ditemukan smoke lokal 31 Okt).
func flowHasSteps(flowJSON string) bool {
	var steps []FlowStep
	return json.Unmarshal([]byte(flowJSON), &steps) == nil && len(steps) > 0
}

// SetResolver - sambungkan resolusi approver ke HC + SSO (dipanggil main.go).
func (s *Service) SetResolver(hcURL, ssoURL, clientID, clientSecret string) {
	s.hcURL = hcURL
	s.ssoURL = ssoURL
	s.clientID = clientID
	s.clientSecret = clientSecret
}

// serviceToken - client credentials ke SSO (pola teamsync).
func (s *Service) serviceToken() (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id":     s.clientID,
		"client_secret": s.clientSecret,
		"grant_type":    "client_credentials",
	})
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Post(s.ssoURL+"/api/oauth/token", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(body, &r)
	if r.Data.AccessToken != "" {
		return r.Data.AccessToken, nil
	}
	return r.AccessToken, nil
}

// resolveSupervisor - HC-3: atasan langsung user sebagai SSO user_id.
func (s *Service) resolveSupervisor(userID string) (string, error) {
	tok, err := s.serviceToken()
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest(http.MethodGet, s.hcURL+"/api/org/direct-supervisor?user_id="+userID, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var env struct {
		Data struct {
			UserID string `json:"user_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", err
	}
	return env.Data.UserID, nil
}

// initApprovalFlow - buat baris ticket_approvals dari approval_flow layanan.
// Rule yang gagal teresolusi tetap dibuat (approver kosong) + event catatan.
// AP-04: tiap baris diberi due_at (kalender jam layanan tim tiket; default 480 menit).
// AP-05: rule parallel membuat satu baris per approver pada step yang sama.
func (s *Service) initApprovalFlow(ticketID, flowJSON, requesterID string, t *entity.Ticket) {
	var steps []FlowStep
	if err := json.Unmarshal([]byte(flowJSON), &steps); err != nil || len(steps) == 0 {
		return
	}
	cal := s.loadCalendarForTeam(t.TeamID)
	for _, st := range steps {
		mins := st.DueMinutes
		if mins <= 0 {
			mins = 480
		}
		due := cal.AddBusinessMinutes(s.now(), mins)
		approvers := []string{}
		mode := ""
		switch st.Rule {
		case "person":
			approvers = []string{st.UserID}
		case "direct_supervisor":
			sv, err := s.resolveSupervisor(requesterID)
			if err == nil && sv != "" {
							if s.isOnLeave(sv) {
				if up, e2 := s.resolveSupervisor(sv); e2 == nil && up != "" && up != sv {
					_ = s.AddSystemEvent(ticketID, "approver sedang cuti - keputusan didelegasikan ke atasan approver (AP-06)", "")
					sv = up
				}
			}
			approvers = []string{sv}
			} else {
				_ = s.AddSystemEvent(ticketID, "approver langkah "+fmt.Sprint(st.Step)+" tidak teresolusi (atasan langsung kosong) - menunggu admin", st.Rule)
			}
		case "parallel":
			mode = st.Mode
			if mode == "" {
				mode = "all"
			}
			approvers = st.Users
		default:
			_ = s.AddSystemEvent(ticketID, "rule approval tidak dikenal: "+st.Rule, "")
			continue
		}
		for _, ap := range approvers {
			row := entity.TicketApproval{
				Base:       entity.Base{},
				TicketID:   ticketID,
				Step:       st.Step,
				Rule:       st.Rule,
				ApproverID: ap,
				Mode:       mode,
			}
			row.DueAt = &due
			_ = s.db.Create(&row).Error
			if ap != "" && t != nil {
				s.notifyInApp(ap, "approval_menunggu", t, "Approval menunggu: "+t.Number, "Tiket '"+t.Subject+"' menunggu keputusan Anda.")
			}
		}
	}
}

// activeStepRows - baris langkah AKTIF (step terendah yang belum tuntas menurut mode:
// any tuntas saat satu keputusan; all tunggu semua; baris kosong = flow selesai) (AP-05).
func (s *Service) activeStepRows(ticketID string) []entity.TicketApproval {
	var all []entity.TicketApproval
	s.db.Where("ticket_id = ?", ticketID).Order("step ASC, created_at ASC").Find(&all)
	byStep := map[int][]entity.TicketApproval{}
	var order []int
	for _, r := range all {
		if _, ok := byStep[r.Step]; !ok {
			order = append(order, r.Step)
		}
		byStep[r.Step] = append(byStep[r.Step], r)
	}
	for _, st := range order {
		rows := byStep[st]
		undecided := 0
		decidedAny := false
		for _, r := range rows {
			if r.Decision == "" {
				undecided++
			} else {
				decidedAny = true
			}
		}
		if undecided == 0 {
			continue
		}
		if rows[0].Mode == "any" && decidedAny {
			continue
		}
		return rows
	}
	return nil
}

// ApprovalInboxItem - baris kotak approval (AP-03).
type ApprovalInboxItem struct {
	ApprovalID string     `json:"approval_id"`
	TicketID   string     `json:"ticket_id"`
	Number     string     `json:"number"`
	Subject    string     `json:"subject"`
	Step       int        `json:"step"`
	Rule       string     `json:"rule"`
	Status     string     `json:"status"`
	DueAt      *time.Time `json:"due_at,omitempty"`
}

// ApprovalInbox - approval menunggu keputusan user (AP-03).
func (s *Service) ApprovalInbox(userID string) ([]ApprovalInboxItem, error) {
	var rows []ApprovalInboxItem
	err := s.db.Table("ticket_approvals ta").
		Select("ta.id as approval_id, ta.ticket_id, t.number, t.subject, ta.step, ta.rule, ta.status, ta.due_at").
		Joins("JOIN tickets t ON t.id = ta.ticket_id").
		Where("ta.approver_id = ? AND (ta.decision IS NULL OR ta.decision = '')", userID).
		Where("t.status IN ?", []string{ticketstate.MenungguApproval, ticketstate.MenungguPemohon}).
		Where("NOT EXISTS (SELECT 1 FROM ticket_approvals x WHERE x.ticket_id = ta.ticket_id AND x.step < ta.step AND (x.decision IS NULL OR x.decision = ''))").
		Order("ta.created_at ASC").Scan(&rows).Error
	return rows, err
}

// Decide - keputusan approver langkah aktif (AP-02): approved | rejected | revision.
// AP-05: mode any = keputusan pertama menuntaskan langkah; mode all = tunggu semua;
// rejected/revision selalu mengakhiri langkah (baris lain ditandai skipped).
func (s *Service) Decide(ticketID, actorID, decision, comment string) (*entity.Ticket, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if t.Status != ticketstate.MenungguApproval && t.Status != ticketstate.MenungguPemohon {
		return nil, fmt.Errorf("%w: tiket tidak dalam status approval", ErrValidation)
	}
	rows := s.activeStepRows(ticketID)
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: tidak ada langkah approval menunggu", ErrValidation)
	}
	var mine *entity.TicketApproval
	among := false
	for i := range rows {
		if rows[i].ApproverID == actorID {
			among = true
			if rows[i].Decision == "" {
				mine = &rows[i]
			}
		}
	}
	if !among || mine == nil {
		return nil, ErrForbidden
	}
	if decision == "rejected" && comment == "" {
		return nil, fmt.Errorf("%w: alasan penolakan wajib", ErrValidation)
	}
	now := s.now()
	_ = s.db.Model(mine).Updates(map[string]interface{}{"decision": decision, "comment": comment, "decided_at": now}).Error
	_ = s.addEvent(ticketID, "approval", "public", actorID, decision, comment)
	s.notifyInApp(t.RequesterUserID, "keputusan_approval", &t, "Keputusan approval "+t.Number+": "+decision, comment)

	mode := rows[0].Mode
	stepDone := false
	outcome := decision
	if mode == "any" || decision == "rejected" || decision == "revision" {
		stepDone = true
		for i := range rows {
			if rows[i].ID != mine.ID && rows[i].Decision == "" {
				_ = s.db.Model(&rows[i]).Updates(map[string]interface{}{"decision": "skipped", "decided_at": now}).Error
			}
		}
	} else {
		pending := 0
		for i := range rows {
			if rows[i].ID != mine.ID && rows[i].Decision == "" {
				pending++
			}
		}
		if pending == 0 {
			stepDone = true
		}
	}
	if stepDone {
		switch outcome {
		case "rejected":
			_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.Ditolak}).Error
		case "revision":
			_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.MenungguPemohon}).Error
		case "approved":
			next := s.activeStepRows(ticketID)
			if len(next) == 0 {
				if t.Status == ticketstate.MenungguPemohon {
					_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.MenungguApproval}).Error
					t.Status = ticketstate.MenungguApproval
				}
				_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.Terbuka}).Error
			}
		}
	}
	_ = s.db.Where("id = ?", ticketID).First(&t).Error
	return &t, nil
}

// isOnLeave - AP-06: user sedang cuti disetujui hari ini? (via HC org/on-leave).
func (s *Service) isOnLeave(userID string) bool {
	tok, err := s.serviceToken()
	if err != nil {
		return false
	}
	req, _ := http.NewRequest(http.MethodGet, s.hcURL+"/api/org/on-leave?user_id="+userID, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var env struct {
		Data struct {
			OnLeave bool `json:"on_leave"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &env) != nil {
		return false
	}
	return env.Data.OnLeave
}
