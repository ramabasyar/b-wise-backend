package ticketsvc

import (
	"encoding/json"
	"fmt"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
)

// ===== Sub-tugas lintas unit (AG-09, KT-08, SL-06) =====

// TaskView - tugas + info tim + hak aktor (untuk panel tiket induk).
type TaskView struct {
	entity.TicketTask
	TeamCode string `json:"team_code"`
	TeamName string `json:"team_name"`
	CanDone  bool   `json:"can_done"`
	Late     bool   `json:"late"`
}

// taskTemplateStep - satu langkah template sub-tugas layanan (KT-08).
type taskTemplateStep struct {
	TeamCode   string `json:"team_code"`
	Title      string `json:"title"`
	OLAMinutes int    `json:"ola_minutes"`
}

func parseTaskTemplate(js string) []taskTemplateStep {
	if js == "" || js == "{}" {
		return nil
	}
	var steps []taskTemplateStep
	if err := json.Unmarshal([]byte(js), &steps); err != nil {
		return nil
	}
	return steps
}

// initTasksFromTemplate - buat sub-tugas otomatis saat tiket dibuat (KT-08).
func (s *Service) initTasksFromTemplate(ticketID, templateJSON string) {
	steps := parseTaskTemplate(templateJSON)
	for i, st := range steps {
		if st.Title == "" {
			continue
		}
		var team entity.Team
		if err := s.db.Where("code = ? AND is_active = ?", st.TeamCode, true).First(&team).Error; err != nil {
			_ = s.AddSystemEvent(ticketID, "template sub-tugas: tim tidak aktif/dikenal "+st.TeamCode, "")
			continue
		}
		s.createTask(ticketID, team, st.Title, st.OLAMinutes, i+1)
	}
	if len(steps) > 0 {
		_ = s.AddSystemEvent(ticketID, fmt.Sprintf("template layanan membuat %d sub-tugas lintas unit", len(steps)), "")
	}
}

func (s *Service) createTask(ticketID string, team entity.Team, title string, olaMinutes, sort int) *entity.TicketTask {
	t := entity.TicketTask{TicketID: ticketID, TeamID: team.ID, Title: title, Status: "open", Sort: sort}
	if olaMinutes > 0 {
		cal := s.loadCalendar(team.CalendarID)
		due := cal.AddBusinessMinutes(s.now(), olaMinutes)
		t.OLADueAt = &due
	}
	if err := s.db.Create(&t).Error; err != nil {
		return nil
	}
	return &t
}

// TasksForViewer - daftar sub-tugas tiket + status OLA + hak aktor (SL-06).
func (s *Service) TasksForViewer(ticketID, userID string) ([]TaskView, error) {
	var rows []entity.TicketTask
	if err := s.db.Where("ticket_id = ?", ticketID).Order("sort ASC, created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	teams := s.TeamsMap()
	now := s.now()
	out := make([]TaskView, 0, len(rows))
	for _, r := range rows {
		v := TaskView{TicketTask: r}
		if t, ok := teams[r.TeamID]; ok {
			v.TeamCode, v.TeamName = t.Code, t.Name
		}
		v.CanDone = r.Status == "open" && s.IsMember(r.TeamID, userID)
		v.Late = r.Status == "open" && r.OLADueAt != nil && now.After(*r.OLADueAt)
		out = append(out, v)
	}
	return out, nil
}

// AddTask - petugas tiket menambah sub-tugas manual ke tim lain (AG-09).
func (s *Service) AddTask(ticketID, actorID, teamCode, title string, olaMinutes int) (*entity.TicketTask, error) {
	var tk entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&tk).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(tk.TeamID, actorID) {
		return nil, ErrForbidden
	}
	if title == "" {
		return nil, fmt.Errorf("%w: judul tugas wajib", ErrValidation)
	}
	var team entity.Team
	if err := s.db.Where("code = ? AND is_active = ?", teamCode, true).First(&team).Error; err != nil {
		return nil, fmt.Errorf("%w: tim tujuan tidak aktif", ErrNotFound)
	}
	var maxSort int
	s.db.Model(&entity.TicketTask{}).Where("ticket_id = ?", ticketID).
		Select("COALESCE(MAX(sort),0)").Scan(&maxSort)
	t := s.createTask(ticketID, team, title, olaMinutes, maxSort+1)
	if t == nil {
		return nil, fmt.Errorf("gagal membuat tugas")
	}
	_ = s.AddSystemEvent(ticketID, "sub-tugas ke "+team.Code+": "+title, "")
	return t, nil
}

// DoneTask - anggota tim pemilik tugas menandakan selesai (AG-09).
func (s *Service) DoneTask(ticketID, taskID, actorID string) (*entity.TicketTask, error) {
	var t entity.TicketTask
	if err := s.db.Where("id = ? AND ticket_id = ?", taskID, ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, actorID) {
		return nil, ErrForbidden
	}
	if t.Status != "open" {
		return &t, nil
	}
	now := s.now()
	if err := s.db.Model(&t).Updates(map[string]interface{}{"status": "selesai", "done_at": now, "assignee_id": actorID}).Error; err != nil {
		return nil, err
	}
	_ = s.AddSystemEvent(ticketID, "sub-tugas selesai: "+t.Title, "")
	_ = s.db.Where("id = ?", taskID).First(&t).Error
	return &t, nil
}

// OpenTaskCount - jumlah sub-tugas belum selesai (guard tiket induk, AG-09).
func (s *Service) OpenTaskCount(ticketID string) int64 {
	var n int64
	s.db.Model(&entity.TicketTask{}).Where("ticket_id = ? AND status = ?", ticketID, "open").Count(&n)
	return n
}
