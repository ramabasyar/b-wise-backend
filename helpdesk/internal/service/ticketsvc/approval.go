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
	"gorm.io/gorm"
)

// FlowStep - satu langkah approval pada konfigurasi layanan (jsonb approval_flow).
// Rule v1: person (user_id eksplisit) | direct_supervisor (resolusi HC-3).
type FlowStep struct {
	Step   int    `json:"step"`
	Rule   string `json:"rule"`
	UserID string `json:"user_id,omitempty"`
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

// resolveSupervisor - HC-3: atasan langsung requester sebagai SSO user_id.
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
func (s *Service) initApprovalFlow(ticketID, flowJSON, requesterID string) {
	var steps []FlowStep
	if err := json.Unmarshal([]byte(flowJSON), &steps); err != nil || len(steps) == 0 {
		return
	}
	for _, st := range steps {
		approver := ""
		switch st.Rule {
		case "person":
			approver = st.UserID
		case "direct_supervisor":
			sv, err := s.resolveSupervisor(requesterID)
			if err == nil && sv != "" {
				approver = sv
			} else {
				_ = s.AddSystemEvent(ticketID, "approver langkah "+fmt.Sprint(st.Step)+" tidak teresolusi (atasan langsung kosong) - menunggu admin", st.Rule)
			}
		default:
			_ = s.AddSystemEvent(ticketID, "rule approval tidak dikenal: "+st.Rule, "")
			continue
		}
		row := entity.TicketApproval{
			Base:      entity.Base{},
			TicketID:  ticketID,
			Step:      st.Step,
			Rule:      st.Rule,
			ApproverID: approver,
		}
		_ = s.db.Create(&row).Error
	}
}

// pendingApproval - langkah approval aktif terendah tanpa keputusan.
func (s *Service) pendingApproval(ticketID string) (*entity.TicketApproval, error) {
	var row entity.TicketApproval
	err := s.db.Where("ticket_id = ? AND (decision IS NULL OR decision = '')", ticketID).
		Order("step ASC").First(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}


// ApprovalInboxItem - baris kotak approval (AP-03).
type ApprovalInboxItem struct {
	ApprovalID string `json:"approval_id"`
	TicketID   string `json:"ticket_id"`
	Number     string `json:"number"`
	Subject    string `json:"subject"`
	Step       int    `json:"step"`
	Rule       string `json:"rule"`
	Status     string `json:"status"`
}

// ApprovalInbox - approval menunggu keputusan user (AP-03).
func (s *Service) ApprovalInbox(userID string) ([]ApprovalInboxItem, error) {
	var rows []ApprovalInboxItem
	err := s.db.Table("ticket_approvals ta").
		Select("ta.id as approval_id, ta.ticket_id, t.number, t.subject, ta.step, ta.rule, t.status").
		Joins("JOIN tickets t ON t.id = ta.ticket_id").
		Where("ta.approver_id = ? AND (ta.decision IS NULL OR ta.decision = '')", userID).
		Where("t.status IN ?", []string{ticketstate.MenungguApproval, ticketstate.MenungguPemohon}).
	Where("NOT EXISTS (SELECT 1 FROM ticket_approvals x WHERE x.ticket_id = ta.ticket_id AND x.step < ta.step AND (x.decision IS NULL OR x.decision = ''))").
		Order("ta.created_at ASC").Scan(&rows).Error
	return rows, err
}

// Decide - keputusan approver langkah aktif (AP-02): approved | rejected | revision.
func (s *Service) Decide(ticketID, actorID, decision, comment string) (*entity.Ticket, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if t.Status != ticketstate.MenungguApproval && t.Status != ticketstate.MenungguPemohon {
		return nil, fmt.Errorf("%w: tiket tidak dalam status approval", ErrValidation)
	}
	pend, err := s.pendingApproval(ticketID)
	if err != nil {
		return nil, err
	}
	if pend == nil {
		return nil, fmt.Errorf("%w: tidak ada langkah approval menunggu", ErrValidation)
	}
	if pend.ApproverID == "" {
		return nil, fmt.Errorf("%w: approver belum teresolusi", ErrForbidden)
	}
	if pend.ApproverID != actorID {
		return nil, ErrForbidden
	}
	if decision == "rejected" && comment == "" {
		return nil, fmt.Errorf("%w: alasan penolakan wajib", ErrValidation)
	}
	now := s.now()
	_ = s.db.Model(pend).Updates(map[string]interface{}{"decision": decision, "comment": comment, "decided_at": now}).Error
	_ = s.addEvent(ticketID, "approval", "public", actorID, decision, comment)

	switch decision {
	case "rejected":
		_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.Ditolak}).Error
	case "revision":
		_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.MenungguPemohon}).Error
	case "approved":
		next, _ := s.pendingApproval(ticketID)
		if next == nil {
			if t.Status == ticketstate.MenungguPemohon {
				_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.MenungguApproval}).Error
				t.Status = ticketstate.MenungguApproval
			}
			_ = s.db.Model(&t).Updates(map[string]interface{}{"status": ticketstate.Terbuka}).Error
		}
	default:
		return nil, fmt.Errorf("%w: keputusan tidak dikenal", ErrValidation)
	}
	_ = s.db.Where("id = ?", ticketID).First(&t).Error
	return &t, nil
}
