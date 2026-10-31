package ticketsvc

import (
	"errors"
	"fmt"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
	"github.com/rama/b-wise/helpdesk/internal/service/slasvc"
	"gorm.io/gorm"
)

// Service — operasi tiket inti F1. Semua perubahan status lewat ValidateTransition.
type Service struct {
	db  *gorm.DB
	now func() time.Time
	// resolusi approver (SetResolver)
	hcURL        string
	ssoURL       string
	clientID     string
	clientSecret string
	secretKey     []byte // kunci induk enkripsi rahasia (env, RH-04)
}

func New(db *gorm.DB) *Service { return &Service{db: db, now: time.Now} }

// jsonbOr - kolom jsonb tidak boleh string kosong (SQLSTATE 22P02; guard pola fix permission-service 13 Sep).
func jsonbOr(v string) string {
	if v == "" {
		return "{}"
	}
	return v
}

// CreateParams — parameter pembuatan tiket (pemohon, tanpa CheckAccess).
type CreateParams struct {
	RequesterID string
	ServiceID   string // kosong = pertanyaan umum -> front office
	Subject     string
	Answers     string // jsonb snapshot
	Channel     string
	OnBehalfBy  string
	RoomRef     string // PM-11: ruang dari QR
}

var (
	ErrValidation = errors.New("validasi gagal")
	ErrNotFound   = errors.New("tidak ditemukan")
	ErrForbidden  = errors.New("akses ditolak")
)

// nextNumber — nomor berikutnya secara atomik (UPSERT RETURNING).
func (s *Service) nextNumber() (string, error) {
	year := s.now().Year()
	var seq int64
	err := s.db.Raw(
		"INSERT INTO ticket_sequences (year, last) VALUES (?, 1) "+
			"ON CONFLICT (year) DO UPDATE SET last = ticket_sequences.last + 1 "+
			"RETURNING last", year,
	).Scan(&seq).Error
	if err != nil {
		return "", err
	}
	return FormatNumber(year, seq), nil
}

// Create — buat tiket: katalog -> tim pemilik layanan; tanpa layanan -> front office (OT-01).
func (s *Service) Create(p CreateParams) (*entity.Ticket, error) {
	if p.Subject == "" || p.RequesterID == "" {
		return nil, fmt.Errorf("%w: subject dan requester wajib", ErrValidation)
	}
	if p.Channel == "" {
		p.Channel = "portal"
	}

	var team entity.Team
	var svcConf string
	var svcSLA string
	var svcApproval string
	var svcTasks string
	var svcFormID string
	if p.ServiceID != "" {
		var svc entity.Service
		if err := s.db.Where("id = ? AND status = ?", p.ServiceID, "published").First(&svc).Error; err != nil {
			return nil, fmt.Errorf("%w: layanan tidak tersedia", ErrNotFound)
		}
		svcConf = svc.Confidentiality
		svcSLA = svc.SLAPolicyID
		svcApproval = svc.ApprovalFlow
		svcTasks = svc.TaskTemplate
		if pub := s.FormPublished(p.ServiceID); pub != nil {
			if err := validateAnswers(pub.Schema, p.Answers); err != nil {
				return nil, err
			}
			svcFormID = pub.ID
		}
		if err := s.db.Where("id = ? AND is_active = ?", svc.TeamID, true).First(&team).Error; err != nil {
			return nil, fmt.Errorf("%w: tim layanan tidak aktif", ErrNotFound)
		}
	} else {
		if err := s.db.Where("is_front_office = ? AND is_active = ?", true, true).First(&team).Error; err != nil {
			return nil, fmt.Errorf("%w: tim front office belum ada", ErrNotFound)
		}
	}

	number, err := s.nextNumber()
	if err != nil {
		return nil, err
	}

	status := ticketstate.Baru
	if flowHasSteps(svcApproval) {
		status = ticketstate.MenungguApproval
	}
	conf := "normal"
	if svcConf != "" {
		conf = svcConf
	}

	t := entity.Ticket{
		Base:            entity.Base{},
		Number:          number,
		Type:            "request",
		ServiceID:       p.ServiceID,
		TeamID:          team.ID,
		RequesterUserID: p.RequesterID,
		OnBehalfBy:      p.OnBehalfBy,
		RoomRef:         p.RoomRef,
		Channel:         p.Channel,
		Subject:         p.Subject,
		Answers:         jsonbOr(p.Answers),
		FormVersionID:   svcFormID,
		Status:          status,
		Confidentiality: conf,
	}
	if svcSLA != "" {
		var pol entity.SLAPolicy
		if err := s.db.Where("id = ?", svcSLA).First(&pol).Error; err == nil {
			cal := s.loadCalendar(team.CalendarID)
			targets, _ := slasvc.ParseTargets(pol.Targets)
			dr, ds := slasvc.DueDates(cal, targets, "normal", s.now())
			t.DueResponseAt, t.DueResolutionAt = dr, ds
		}
	}
	if err := s.db.Create(&t).Error; err != nil {
		return nil, err
	}
	_ = s.AddSystemEvent(t.ID, "tiket dibuat", team.Code)
	if svcTasks != "" && svcTasks != "{}" {
		s.initTasksFromTemplate(t.ID, svcTasks)
	}
	s.notifyInApp(p.RequesterID, "tiket_dibuat", &t, "Tiket "+t.Number+" dibuat", "Permintaan Anda diterima dan masuk antrean "+team.Code+".")
	if flowHasSteps(svcApproval) {
		s.initApprovalFlow(t.ID, svcApproval, p.RequesterID, &t)
	}
	return &t, nil
}

// AddSystemEvent — event sistem tanpa aktor.
func (s *Service) AddSystemEvent(ticketID, body, meta string) error {
	return s.addEvent(ticketID, "system", "public", "", body, meta)
}

// addEvent — timeline append-only.
func (s *Service) addEvent(ticketID, kind, visibility, authorID, body, meta string) error {
	ev := entity.TicketEvent{
		Base:       entity.Base{},
		TicketID:   ticketID,
		Kind:       kind,
		Visibility: visibility,
		AuthorID:   authorID,
		Body:       body,
		Meta:       jsonbOr(meta),
	}
	return s.db.Create(&ev).Error
}

// GetForViewer — otorisasi 9.3: pemohon ATAU anggota aktif tim pemilik.
func (s *Service) GetForViewer(ticketID, userID string) (*entity.Ticket, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if t.RequesterUserID == userID {
		return &t, nil
	}
	var m entity.TeamMember
	err := s.db.Where("team_id = ? AND user_id = ? AND is_active = ?", t.TeamID, userID, true).First(&m).Error
	if err == nil {
		return &t, nil
	}
	// approver pending juga boleh melihat (AP-03)
	var ap entity.TicketApproval
	err = s.db.Where("ticket_id = ? AND approver_id = ? AND (decision IS NULL OR decision = '')", t.ID, userID).First(&ap).Error
	if err == nil {
		return &t, nil
	}
	return nil, ErrForbidden
}

// Events — timeline tiket (pemohon hanya melihat visibility public).
func (s *Service) Events(ticketID string, includeInternal bool) ([]entity.TicketEvent, error) {
	q := s.db.Where("ticket_id = ?", ticketID)
	if !includeInternal {
		q = q.Where("visibility = ?", "public")
	}
	var list []entity.TicketEvent
	err := q.Order("created_at ASC").Find(&list).Error
	return list, err
}

// ListMine — tiket pemohon (aturan 1: filter di query, bukan UI).
func (s *Service) ListMine(userID string) ([]entity.Ticket, error) {
	var list []entity.Ticket
	err := s.db.Where("requester_user_id = ?", userID).Order("created_at DESC").Find(&list).Error
	return list, err
}

// Queue — antrean tim petugas (AG-01).
func (s *Service) Queue(teamID, filter, userID string) ([]entity.Ticket, error) {
	q := s.db.Where("team_id = ?", teamID)
	switch filter {
	case "unassigned":
		q = q.Where("assignee_id IS NULL OR assignee_id = ''")
	case "mine":
		q = q.Where("assignee_id = ?", userID)
	case "open":
		q = q.Where("status IN ?", []string{ticketstate.Baru, ticketstate.Terbuka, ticketstate.MenungguApproval})

	case "waiting":
		q = q.Where("status IN ?", []string{ticketstate.MenungguPemohon, ticketstate.MenungguPihakLain})
	}
	var list []entity.Ticket
	err := q.Order("created_at ASC").Find(&list).Error
	return list, err
}

// TeamOf — tim aktif tempat user menjadi anggota (untuk antrean petugas).
func (s *Service) TeamOf(userID string) (*entity.Team, error) {
	var m entity.TeamMember
	if err := s.db.Where("user_id = ? AND is_active = ?", userID, true).First(&m).Error; err != nil {
		return nil, fmt.Errorf("%w: bukan anggota tim mana pun", ErrForbidden)
	}
	var t entity.Team
	if err := s.db.Where("id = ? AND is_active = ?", m.TeamID, true).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	return &t, nil
}

// IsMember — cek keanggotaan aktif tim.
func (s *Service) IsMember(teamID, userID string) bool {
	var m entity.TeamMember
	return s.db.Where("team_id = ? AND user_id = ? AND is_active = ?", teamID, userID, true).First(&m).Error == nil
}

// AddEvent — balasan publik / catatan internal; SL-07: balasan publik pertama
// petugas tercatat sebagai respons pertama.
func (s *Service) AddEvent(ticketID, actorID, kind, visibility, body string) (*entity.Ticket, error) {
	if body == "" {
		return nil, fmt.Errorf("%w: isi pesan wajib", ErrValidation)
	}
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if err := s.addEvent(ticketID, kind, visibility, actorID, body, ""); err != nil {
		return nil, err
	}
	if kind == "reply" && visibility == "public" {
		if actorID == t.RequesterUserID {
			s.notifyInApp(t.AssigneeID, "balasan_pemohon", &t, "Pemohon membalas "+t.Number, body)
		} else {
			s.notifyInApp(t.RequesterUserID, "balasan_petugas", &t, "Balasan baru di "+t.Number, body)
		}
	}
	if kind == "reply" && visibility == "public" && t.FirstResponseAt == nil {
		now := s.now()
		if err := s.db.Model(&t).Update("first_response_at", now).Error; err != nil {
			return nil, err
		}
		t.FirstResponseAt = &now
	}
	return &t, nil
}

// ChangeStatus — ubah status dengan guard state machine + timestamp.
func (s *Service) ChangeStatus(ticketID, actorID, to, note string) (*entity.Ticket, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	now := s.now()
	var resolvedAt time.Time
	if t.ResolvedAt != nil {
		resolvedAt = *t.ResolvedAt
	}
	if to == ticketstate.Selesai {
		if open := s.OpenTaskCount(ticketID); open > 0 {
			return nil, fmt.Errorf("%w: masih ada %d sub-tugas lintas unit yang belum selesai", ErrValidation, open)
		}
	}
	if err := ValidateTransition(t.Status, to, resolvedAt, now); err != nil {
		return nil, err
	}
	updates := map[string]interface{}{"status": to}
	switch to {
	case ticketstate.Selesai:
		updates["resolved_at"] = now
	case ticketstate.Ditutup:
		updates["closed_at"] = now
	case ticketstate.Dikerjakan:
		if t.Status == ticketstate.Selesai {
			updates["resolved_at"] = nil
		}
	}
	if ticketstate.PausesSLA(to) {
		updates["paused_at"] = now
	} else if ticketstate.PausesSLA(t.Status) && t.PausedAt != nil {
		updates["paused_at"] = nil
		cal := s.loadCalendarForTeam(t.TeamID)
		mins := cal.BusinessMinutesBetween(*t.PausedAt, now)
		if mins > 0 {
			if t.DueResponseAt != nil {
				updates["due_response_at"] = cal.AddBusinessMinutes(*t.DueResponseAt, mins)
			}
			if t.DueResolutionAt != nil {
				updates["due_resolution_at"] = cal.AddBusinessMinutes(*t.DueResolutionAt, mins)
			}
		}
	}
	if err := s.db.Model(&t).Updates(updates).Error; err != nil {
		return nil, err
	}
	_ = s.addEvent(ticketID, "status", "public", actorID, to, note)
	if actorID != t.RequesterUserID {
		s.notifyInApp(t.RequesterUserID, "status_tiket", &t, "Status "+t.Number+": "+to, "Status tiket Anda kini: "+to)
	}
	return &t, nil
}

// Assign — tugaskan petugas (AG-03).
func (s *Service) Assign(ticketID, actorID, assigneeID string) (*entity.Ticket, error) {
	if assigneeID == "" {
		return nil, fmt.Errorf("%w: assignee wajib", ErrValidation)
	}
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, actorID) || !s.IsMember(t.TeamID, assigneeID) {
		return nil, ErrForbidden
	}
	if err := s.db.Model(&t).Update("assignee_id", assigneeID).Error; err != nil {
		return nil, err
	}
	_ = s.addEvent(ticketID, "assign", "internal", actorID, assigneeID, "")
	return &t, nil
}

// Take — petugas mengambil sendiri (AG-03); Terbuka otomatis jadi Dikerjakan.
func (s *Service) Take(ticketID, userID string) (*entity.Ticket, error) {
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, userID) {
		return nil, ErrForbidden
	}
	if t.Status == ticketstate.Terbuka {
		if err := ValidateTransition(t.Status, ticketstate.Dikerjakan, time.Time{}, s.now()); err != nil {
			return nil, err
		}
		_ = s.db.Model(&t).Updates(map[string]interface{}{"assignee_id": userID, "status": ticketstate.Dikerjakan})
	} else {
		_ = s.db.Model(&t).Update("assignee_id", userID)
	}
	_ = s.addEvent(ticketID, "assign", "internal", userID, userID, "ambil sendiri")
	return &t, nil
}

// Transfer — pindah tim, alasan wajib; SLA pemohon TIDAK di-reset (D-11).
func (s *Service) Transfer(ticketID, actorID, toTeamID, reason string) (*entity.Ticket, error) {
	if reason == "" {
		return nil, fmt.Errorf("%w: alasan transfer wajib", ErrValidation)
	}
	var toTeam entity.Team
	if err := s.db.Where("id = ? AND is_active = ?", toTeamID, true).First(&toTeam).Error; err != nil {
		return nil, fmt.Errorf("%w: tim tujuan tidak aktif", ErrNotFound)
	}
	var t entity.Ticket
	if err := s.db.Where("id = ?", ticketID).First(&t).Error; err != nil {
		return nil, ErrNotFound
	}
	if !s.IsMember(t.TeamID, actorID) {
		return nil, ErrForbidden
	}
	if t.TeamID == toTeamID {
		return nil, fmt.Errorf("%w: tim tujuan sama dengan tim sekarang", ErrValidation)
	}
	if err := s.db.Model(&t).Updates(map[string]interface{}{"team_id": toTeamID, "assignee_id": ""}).Error; err != nil {
		return nil, err
	}
	_ = s.addEvent(ticketID, "transfer", "public", actorID, toTeam.Code, reason)
	return &t, nil
}

// Teams - daftar tim aktif (untuk pilihan transfer).
func (s *Service) Teams() ([]entity.Team, error) {
	var list []entity.Team
	err := s.db.Where("is_active = ?", true).Order("code ASC").Find(&list).Error
	return list, err
}

// TeamsMap - peta id -> tim aktif (pengayaan respons sub-tugas).
func (s *Service) TeamsMap() map[string]entity.Team {
	var list []entity.Team
	s.db.Where("is_active = ?", true).Find(&list)
	m := make(map[string]entity.Team, len(list))
	for _, t := range list {
		m[t.ID] = t
	}
	return m
}

// TeamMembers - anggota aktif sebuah tim (untuk pilihan penugasan).
func (s *Service) TeamMembers(teamID string) ([]entity.TeamMember, error) {
	var list []entity.TeamMember
	err := s.db.Where("team_id = ? AND is_active = ?", teamID, true).Find(&list).Error
	return list, err
}

// loadCalendar - kalender bisnis tim (fallback: kalender aktif pertama / default Sen-Jum 08-16).
func (s *Service) loadCalendar(calendarID string) *slasvc.Calendar {
	var row entity.BusinessCalendar
	q := s.db.Where("is_active = ?", true)
	if calendarID != "" {
		q = s.db.Where("id = ? AND is_active = ?", calendarID, true)
	}
	if err := q.First(&row).Error; err != nil {
		iv := []slasvc.Interval{{Start: "08:00", End: "16:00"}}
		return &slasvc.Calendar{Weekly: map[string][]slasvc.Interval{"mon": iv, "tue": iv, "wed": iv, "thu": iv, "fri": iv}, Timezone: "Asia/Jakarta"}
	}
	cal, _ := slasvc.ParseCalendar(row.WeeklyHours, row.Timezone)
	if cal == nil {
		iv := []slasvc.Interval{{Start: "08:00", End: "16:00"}}
		return &slasvc.Calendar{Weekly: map[string][]slasvc.Interval{"mon": iv, "tue": iv, "wed": iv, "thu": iv, "fri": iv}, Timezone: "Asia/Jakarta"}
	}
	return cal
}

func (s *Service) loadCalendarForTeam(teamID string) *slasvc.Calendar {
	var tm entity.Team
	if err := s.db.Where("id = ?", teamID).First(&tm).Error; err == nil {
		return s.loadCalendar(tm.CalendarID)
	}
	return s.loadCalendar("")
}
