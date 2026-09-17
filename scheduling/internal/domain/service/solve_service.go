package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	entity "github.com/rama/b-wise/scheduling/internal/domain/entity"
	"github.com/rama/b-wise/scheduling/internal/service/solverclient"
	"gorm.io/gorm"
)

// ==================== SOLVE SERVICE (F1) ====================
// Job async: build model dari master data → stream sidecar → persist entries.
// Progress ditulis per event; UI polling GET /api/solve-jobs/:id.

type SolveService struct {
	db     *gorm.DB
	solver *solverclient.SolverClient
	cancel map[string]context.CancelFunc
}

func NewSolveService(db *gorm.DB, solver *solverclient.SolverClient) *SolveService {
	return &SolveService{db: db, solver: solver, cancel: map[string]context.CancelFunc{}}
}

var (
	ErrNoData     = errors.New("tidak ada data untuk dijadwalkan")
	ErrJobRunning = errors.New("masih ada job berjalan untuk semester ini")
)

// Start — buat job + jalankan background goroutine.
func (s *SolveService) Start(termID, actor string, timeLimit int) (*entity.SolveJob, error) {
	// tolak kalau masih ada job aktif utk term sama
	var active int64
	s.db.Model(&entity.SolveJob{}).
		Where("term_id = ? AND status NOT IN ('done','failed','cancelled')", termID).Count(&active)
	if active > 0 {
		return nil, ErrJobRunning
	}
	if timeLimit <= 0 || timeLimit > 600 {
		timeLimit = 60
	}
	job := &entity.SolveJob{TermID: termID, Status: "queued", Progress: 0,
		Phase: "queued", Message: "Menunggu dimulai", TimeLimit: timeLimit, StartedBy: actor}
	if err := s.db.Create(job).Error; err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel[job.ID] = cancel
	go s.run(ctx, job.ID)
	return job, nil
}

func (s *SolveService) Cancel(jobID string) error {
	var job entity.SolveJob
	if err := s.db.Where("id = ?", jobID).First(&job).Error; err != nil {
		return ErrNotFound
	}
	if job.Status == "done" || job.Status == "failed" || job.Status == "cancelled" {
		return fmt.Errorf("%w: job sudah %s", ErrValidation, job.Status)
	}
	if c, ok := s.cancel[jobID]; ok {
		c()
	}
	now := time.Now()
	s.db.Model(&job).Updates(map[string]any{"status": "cancelled", "progress": 100, "phase": "cancelled",
		"message": "Dibatalkan oleh user", "finished_at": now})
	return nil
}

func (s *SolveService) Get(jobID string) (*entity.SolveJob, error) {
	var j entity.SolveJob
	if err := s.db.Preload("Term").Where("id = ?", jobID).First(&j).Error; err != nil {
		return nil, ErrNotFound
	}
	return &j, nil
}

func (s *SolveService) List(termID string) ([]entity.SolveJob, error) {
	q := s.db.Model(&entity.SolveJob{}).Preload("Term").Order("created_at DESC").Limit(50)
	if termID != "" {
		q = q.Where("term_id = ?", termID)
	}
	var list []entity.SolveJob
	return list, q.Find(&list).Error
}

func (s *SolveService) patch(jobID string, fields map[string]any) {
	if err := s.db.Model(&entity.SolveJob{}).Where("id = ?", jobID).Updates(fields).Error; err != nil {
		log.Printf("[solve] patch job %s gagal: %v", jobID, err)
	}
}

// buildModel — kumpulkan data master → payload solver.
func (s *SolveService) buildModel(termID string) (*solverclient.SolverPayload, error) {
	var offerings []entity.Offering
	if err := s.db.Preload("Course").Preload("Lecturer").Preload("ClassGroup").
		Where("term_id = ? AND is_active = ?", termID, true).Find(&offerings).Error; err != nil {
		return nil, err
	}
	if len(offerings) == 0 {
		return nil, ErrNoData
	}
	var rooms []entity.Room
	if err := s.db.Where("is_active = ?", true).Find(&rooms).Error; err != nil {
		return nil, err
	}
	var roomTypes []entity.RoomType
	if err := s.db.Where("is_active = ?", true).Order("sort_order ASC").Find(&roomTypes).Error; err != nil {
		return nil, err
	}
	var slots []entity.TimeSlot
	if err := s.db.Where("is_active = ?", true).Order("day ASC, \"order\" ASC").Find(&slots).Error; err != nil {
		return nil, err
	}
	if len(rooms) == 0 || len(slots) == 0 {
		return nil, ErrNoData
	}
	var blocks []entity.LecturerAvailability
	s.db.Where("mode = ?", "blocked").Find(&blocks)

	// kamus jenis MK → kebutuhan ruang (theory|practice|any|none)
	var courseTypes []entity.CourseType
	s.db.Where("is_active = ?", true).Find(&courseTypes)
	needOf := map[string]string{}
	for _, ct := range courseTypes {
		needOf[ct.Code] = ct.RoomNeed
	}
	resolveNeed := func(courseType string) string {
		if n, ok := needOf[courseType]; ok && n != "" {
			return n
		}
		// fallback perilaku lama utk code tanpa kamus (data pra-migrasi)
		if courseType == "practice" {
			return "practice"
		}
		return "theory"
	}

	// F-multi-dosen: muat relasi pengampu per offering (offering_lecturers).
	offIDs := make([]string, 0, len(offerings))
	for _, o := range offerings {
		offIDs = append(offIDs, o.ID)
	}
	var offLects []entity.OfferingLecturer
	if len(offIDs) > 0 {
		s.db.Where("offering_id IN ?", offIDs).Order("sort_order ASC").Find(&offLects)
	}
	lectsByOff := map[string][]entity.OfferingLecturer{}
	for _, ol := range offLects {
		lectsByOff[ol.OfferingID] = append(lectsByOff[ol.OfferingID], ol)
	}
	// constraintLects — dosen yang HARUS bebas slot untuk offering ini:
	//   parallel       → SEMUA dosen (team teaching hadir bersama di sesi sama);
	//   single/split_* → dosen utama saja (paruh dosen split tidak selalu mengajar,
	//                    info sesi belum granular — dosen utama mewakili).
	constraintLects := func(o entity.Offering) []string {
		rels := lectsByOff[o.ID]
		for _, r := range rels {
			if r.Pattern == "parallel" {
				ids := make([]string, 0, len(rels))
				for _, r2 := range rels {
					ids = append(ids, r2.LecturerID)
				}
				return ids
			}
		}
		if o.LecturerID != "" {
			return []string{o.LecturerID}
		}
		if len(rels) > 0 {
			return []string{rels[0].LecturerID}
		}
		return nil
	}

	p := &solverclient.SolverPayload{Rooms: make([]solverclient.SolverRoom, 0, len(rooms)),
		RoomTypes:       make([]solverclient.SolverRoomType, 0, len(roomTypes)),
		Slots:          make([]solverclient.SolverSlot, 0, len(slots)),
		LecturerBlocks: make([]solverclient.SolverBlock, 0, len(blocks))}
	for _, rt := range roomTypes {
		p.RoomTypes = append(p.RoomTypes, solverclient.SolverRoomType{Code: rt.Code, ForTheory: rt.ForTheory, ForPractice: rt.ForPractice})
	}
	// bobot soft constraint (default bila baris config belum ada)
	var cfg entity.SolveConfig
	if err := s.db.Where("code = ?", "default").First(&cfg).Error; err != nil {
		cfg = entity.SolveConfig{SpreadWeight: 5, RoomWasteWeight: 2, LastSlotWeight: 1}
	}
	p.SolveConfig = &solverclient.SolverConfig{
		SpreadWeight: cfg.SpreadWeight, RoomWasteWeight: cfg.RoomWasteWeight, LastSlotWeight: cfg.LastSlotWeight,
	}
	for _, r := range rooms {
		p.Rooms = append(p.Rooms, solverclient.SolverRoom{ID: r.ID, Code: r.Code, Type: r.Type, Capacity: r.Capacity})
	}
	for _, sl := range slots {
		p.Slots = append(p.Slots, solverclient.SolverSlot{ID: sl.ID, Day: sl.Day, Order: sl.Order, StartTime: sl.StartTime, EndTime: sl.EndTime})
	}
	for _, b := range blocks {
		p.LecturerBlocks = append(p.LecturerBlocks, solverclient.SolverBlock{LecturerID: b.LecturerID, Day: b.Day})
	}
	// kebijakan durasi SKS (default Permendikbud 50/170)
	var tp entity.TimePolicy
	if err := s.db.Where("code = ?", "default").First(&tp).Error; err != nil || tp.SksMinutesTheory <= 0 {
		tp = entity.TimePolicy{SksMinutesTheory: 50, SksMinutesPractice: 170}
	}
	for _, o := range offerings {
		ctype := "theory"
		if o.Course != nil {
			ctype = o.Course.Type
		}
		// MK tanpa ruang fisik (online/kerja lapangan) — tidak dikirim ke solver
		if o.RoomType == "" && resolveNeed(ctype) == "none" {
			continue
		}
		// F-multi-dosen: offering tanpa sesi terjadwalkan sama sekali (semua SKS lapangan/simulasi) — skip
		if o.SessionSks <= 0 && o.PracticeSks <= 0 {
			continue
		}
		cLects := constraintLects(o)
		sessions := o.SessionsPerWeek
		if sessions <= 0 {
			sessions = 1
		}
		// practice-only (semua SKS praktikum, tanpa sesi teori) — hanya sesi praktikum
		if o.SessionSks <= 0 && o.PracticeSks > 0 {
			sessions = 0
		}
		sksSesi := o.SessionSks
		if sksSesi <= 0 {
			sksSesi = 2
		}
		dur := (sksSesi + 1) / 2 // fallback slot (payload lama tanpa duration_minutes)
		theoryMin := sksSesi * tp.SksMinutesTheory
		for n := 1; n <= sessions; n++ {
			lect := ""
			if o.Lecturer != nil {
				lect = o.Lecturer.ID
			}
			ccode, cneed := "", "theory"
			if o.Course != nil {
				ccode = o.Course.Code
				cneed = resolveNeed(o.Course.Type)
			}
			size := 0
			if o.ClassGroup != nil {
				size = o.ClassGroup.SizeEst
			}
				p.Sessions = append(p.Sessions, solverclient.SolverSession{
					Key: fmt.Sprintf("%s#%d", o.ID, n), OfferingID: o.ID,
					CourseCode: ccode, CourseType: ctype, RoomNeed: cneed, RoomType: o.RoomType,
					GroupID: o.ClassGroupID, GroupSize: size, LecturerID: lect, LecturerIDs: cLects,
					DurationSlots: dur, DurationMinutes: theoryMin,
				})
		}
		// sesi praktikum terpisah (ruang for_practice, durasi blok sks × menit praktikum)
		if o.PracticeSks > 0 {
			lect := ""
			if o.Lecturer != nil {
				lect = o.Lecturer.ID
			}
			ccode := ""
			if o.Course != nil {
				ccode = o.Course.Code
			}
			size := 0
			if o.ClassGroup != nil {
				size = o.ClassGroup.SizeEst
			}
			practiceMin := o.PracticeSks * tp.SksMinutesPractice
			p.Sessions = append(p.Sessions, solverclient.SolverSession{
				Key: fmt.Sprintf("%s#p", o.ID), OfferingID: o.ID,
				CourseCode: ccode, CourseType: "practice", RoomNeed: "practice", RoomType: "",
				GroupID: o.ClassGroupID, GroupSize: size, LecturerID: lect, LecturerIDs: cLects,
				DurationSlots: (o.PracticeSks + 1) / 2, DurationMinutes: practiceMin,
			})
		}
	}
	return p, nil
}

// run — goroutine job.
func (s *SolveService) run(ctx context.Context, jobID string) {
	defer delete(s.cancel, jobID)
	var job entity.SolveJob
	if err := s.db.Where("id = ?", jobID).First(&job).Error; err != nil {
		return
	}
	s.patch(jobID, map[string]any{"status": "modelling", "phase": "modelling", "progress": 5,
		"message": "Mengumpulkan data master…"})

	p, err := s.buildModel(job.TermID)
	if err != nil {
		s.patch(jobID, map[string]any{"status": "failed", "progress": 100, "phase": "failed",
			"message": fmt.Sprintf("Gagal menyiapkan model: %v", err), "finished_at": time.Now()})
		return
	}
	p.JobID = jobID
	p.TimeLimitSeconds = job.TimeLimit

	// locked entries dari hasil terakhir (F2 re-solve) — kirim apa adanya
	var locked []entity.TimetableEntry
	s.db.Where("term_id = ? AND locked = ?", job.TermID, true).Find(&locked)
	for _, l := range locked {
		p.Locked = append(p.Locked, map[string]any{"session_key": l.SessionKey, "slot_id": l.SlotID, "room_id": l.RoomID})
	}

	first := true
	terminal := false // true bila solver mengirim event done/failed (F-perf guard)
	err = s.solver.StreamSolve(ctx, *p, func(ev solverclient.SolverEvent) error {
		if first {
			s.patch(jobID, map[string]any{"status": "solving"})
			first = false
		}
		// simpan progress (throttle: hanya kalau berubah)
		s.patch(jobID, map[string]any{"phase": ev.Phase, "progress": ev.Progress, "message": ev.Message})
		if ev.Phase == "done" {
			// persist entries (replace draft lama job lain di term ini)
			now := time.Now()
			// F3: hapus hanya DRAFT (version_id NULL) — entri milik versi published immutable
			_ = s.db.Where("term_id = ? AND version_id IS NULL AND locked = ?", job.TermID, false).
				Delete(&entity.TimetableEntry{}).Error
			var kept []entity.TimetableEntry
			s.db.Where("term_id = ? AND version_id IS NULL AND locked = ?", job.TermID, true).Find(&kept) // locked draft tetap
			lockedKeys := map[string]bool{}
			for _, l := range kept {
				lockedKeys[l.SessionKey] = true
			}
			for _, a := range ev.Assignments {
				if lockedKeys[str(a["session_key"])] {
					continue // session terkunci sudah ada (dipertahankan) — jangan dobel
				}
				e := entity.TimetableEntry{
					JobID: jobID, TermID: job.TermID,
					OfferingID: str(a["offering_id"]), SessionKey: str(a["session_key"]),
					CourseCode: str(a["course_code"]), GroupID: str(a["group_id"]),
					LecturerID: str(a["lecturer_id"]), SlotID: str(a["slot_id"]), RoomID: str(a["room_id"]),
					Day: intOf(a["day"]), SlotOrder: intOf(a["slot_order"]),
					StartTime: str(a["start_time"]), EndTime: str(a["end_time"]),
				}
				s.db.Create(&e)
			}
			_ = kept
			s.patch(jobID, map[string]any{"status": "done", "stats": ev.Stats, "elapsed": ev.Elapsed, "finished_at": now})
			terminal = true
		}
		if ev.Phase == "failed" {
			s.patch(jobID, map[string]any{"status": "failed", "stats": ev.Stats, "finished_at": time.Now()})
			terminal = true
		}
		// cek cancel
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	})
	if err != nil && ctx.Err() == nil {
		s.patch(jobID, map[string]any{"status": "failed", "message": fmt.Sprintf("Solver gagal: %v", err), "finished_at": time.Now()})
	} else if err == nil && !terminal && ctx.Err() == nil {
		// EOF tanpa event done/failed — solver crash diam-diam; jangan biarkan job menggantung
		s.patch(jobID, map[string]any{"status": "failed", "progress": 100, "phase": "failed",
			"message": "Stream solver berakhir tanpa hasil (sidecar error) — cek log solver", "finished_at": time.Now()})
	}
}

// SetLocked — kunci/buka satu entry (F2). Entry terkunci dipertahankan solver saat re-solve.
func (s *SolveService) SetLocked(entryID string, locked bool) (*entity.TimetableEntry, error) {
	var e entity.TimetableEntry
	if err := s.db.Where("id = ?", entryID).First(&e).Error; err != nil {
		return nil, ErrNotFound
	}
	if err := s.db.Model(&e).Update("locked", locked).Error; err != nil {
		return nil, err
	}
	s.db.Where("id = ?", entryID).First(&e)
	return &e, nil
}

// Entries — hasil jadwal term (latest job / locked).
func (s *SolveService) Entries(termID string) ([]entity.TimetableEntry, error) {
	var list []entity.TimetableEntry
	q := s.db.Preload("Offering").Preload("Offering.Course").Preload("Offering.Lecturer").
		Preload("Room").Preload("ClassGroup").Where("term_id = ?", termID).
		Order("day ASC, slot_order ASC")
	return list, q.Find(&list).Error
}

// Publish — beku draft jadi versi published (F3). Entries distamp version_id (immutable).
func (s *SolveService) Publish(termID, name, note, actor string) (*entity.TimetableVersion, error) {
	var drafts []entity.TimetableEntry
	if err := s.db.Where("term_id = ? AND version_id IS NULL", termID).Find(&drafts).Error; err != nil {
		return nil, err
	}
	if len(drafts) == 0 {
		return nil, fmt.Errorf("%w: tidak ada draft jadwal untuk dipublish (jalankan penjadwalan dulu)", ErrNoData)
	}
	// archive versi published lama term ini
	s.db.Model(&entity.TimetableVersion{}).Where("term_id = ? AND status = ?", termID, "published").
		Update("status", "archived")

	now := time.Now()
	v := &entity.TimetableVersion{TermID: termID, JobID: drafts[0].JobID, Name: name,
		Status: "published", EntriesCount: len(drafts), Note: note, PublishedBy: actor, PublishedAt: &now}
	if err := s.db.Create(v).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&entity.TimetableEntry{}).
		Where("term_id = ? AND version_id IS NULL", termID).
		Update("version_id", v.ID).Error; err != nil {
		return nil, err
	}
	s.fireWebhook(v, drafts)
	return v, nil
}

// Versions — daftar versi per term.
func (s *SolveService) Versions(termID string) ([]entity.TimetableVersion, error) {
	q := s.db.Model(&entity.TimetableVersion{}).Order("created_at DESC").Limit(50)
	if termID != "" {
		q = q.Where("term_id = ?", termID)
	}
	var list []entity.TimetableVersion
	return list, q.Find(&list).Error
}

// PublishedEntries — entri versi published aktif (untuk consumer API).
func (s *SolveService) PublishedEntries(termID string) ([]entity.TimetableEntry, error) {
	var v entity.TimetableVersion
	if err := s.db.Where("term_id = ? AND status = ?", termID, "published").First(&v).Error; err != nil {
		return nil, ErrNotFound
	}
	var list []entity.TimetableEntry
	return list, s.db.Where("version_id = ?", v.ID).Order("day ASC, slot_order ASC").Find(&list).Error
}

// RoomScheduleDay — okupansi satu ruang sehari (consumer/doorlock): entri published term aktif.
func (s *SolveService) RoomScheduleDay(roomID string, day int) ([]map[string]any, error) {
	var terms []entity.Term
	s.db.Where("is_active = ?", true).Order("start_date DESC").Find(&terms)
	for _, t := range terms {
		entries, err := s.PublishedEntries(t.ID)
		if err != nil {
			continue
		}
		out := []map[string]any{}
		for _, e := range entries {
			if e.RoomID == roomID && e.Day == day {
				out = append(out, map[string]any{
					"course_code": e.CourseCode, "group_id": e.GroupID,
					"start_time": e.StartTime, "end_time": e.EndTime, "slot_order": e.SlotOrder,
				})
			}
		}
		return out, nil
	}
	return []map[string]any{}, nil
}

// RoomNow — siapa pakai ruang ini SEKARANG (doorlock realtime).
func (s *SolveService) RoomNow(roomID string) (map[string]any, error) {
	day := int(time.Now().Weekday())
	if day == 0 {
		day = 7 // Senin=1..Minggu=7 (TimeSlot convention)
	}
	hhmm := time.Now().Format("15:04")
	sched, _ := s.RoomScheduleDay(roomID, day)
	for _, e := range sched {
		st, _ := e["start_time"].(string)
		en, _ := e["end_time"].(string)
		if st != "" && en != "" && hhmm >= st && hhmm <= en {
			return map[string]any{"occupied": true, "current": e, "day": day, "time": hhmm}, nil
		}
	}
	return map[string]any{"occupied": false, "day": day, "time": hhmm}, nil
}

// fireWebhook — best effort: umumkan publish ke konsumen terdaftar (env).
func (s *SolveService) fireWebhook(v *entity.TimetableVersion, entries []entity.TimetableEntry) {
	url := os.Getenv("SCHEDULE_WEBHOOK_URL")
	if url == "" {
		log.Printf("[publish] versi %s dipublish (%d entri) — webhook tidak dikonfigurasi", v.ID, len(entries))
		return
	}
	body := map[string]any{
		"event": "timetable.published", "version_id": v.ID, "term_id": v.TermID,
		"name": v.Name, "entries_count": v.EntriesCount, "published_at": v.PublishedAt,
	}
	go func() {
		b, _ := json.Marshal(body)
		resp, err := http.Post(url, "application/json", bytes.NewReader(b))
		if err != nil {
			log.Printf("[publish] webhook gagal: %v", err)
			return
		}
		resp.Body.Close()
		log.Printf("[publish] webhook terkirim -> %s", url)
	}()
}

// TimetableView — entries utk view per dosen/rombel/ruang (gap#3).
// Sumber: versi published; fallback draft bila belum ada versi.
func (s *SolveService) TimetableView(termID, by, id string) ([]map[string]any, error) {
	entries, err := s.PublishedEntries(termID)
	if err != nil || len(entries) == 0 {
		entries, err = s.Entries(termID)
		if err != nil {
			return nil, err
		}
	}
	roomCodes := map[string]string{}
	var rooms []entity.Room
	s.db.Find(&rooms)
	for _, r := range rooms {
		roomCodes[r.ID] = r.Code
	}
	groupCodes := map[string]string{}
	var gs []entity.ClassGroup
	s.db.Find(&gs)
	for _, g := range gs {
		groupCodes[g.ID] = g.Code
	}
	lectNames := map[string]string{}
	var ls []entity.Lecturer
	s.db.Find(&ls)
	for _, l := range ls {
		lectNames[l.ID] = l.Name
	}
	var offs []entity.Offering
	s.db.Preload("Course").Preload("Lecturer").Where("term_id = ?", termID).Find(&offs)
	offByID := map[string]entity.Offering{}
	for _, o := range offs {
		offByID[o.ID] = o
	}
	out := []map[string]any{}
	for _, e := range entries {
		switch by {
		case "lecturer":
			if e.LecturerID != id {
				continue
			}
		case "group":
			if e.GroupID != id {
				continue
			}
		case "room":
			if e.RoomID != id {
				continue
			}
		}
		o := offByID[e.OfferingID]
		cname := ""
		if o.Course != nil {
			cname = o.Course.Name
		}
		lect := lectNames[e.LecturerID]
		if lect == "" && o.Lecturer != nil {
			lect = o.Lecturer.Name
		}
		out = append(out, map[string]any{
			"day": e.Day, "slot_order": e.SlotOrder,
			"start_time": e.StartTime, "end_time": e.EndTime,
			"course_code": e.CourseCode, "course_name": cname,
			"group_code": groupCodes[e.GroupID], "lecturer": lect,
			"room_code": roomCodes[e.RoomID], "locked": e.Locked,
		})
	}
	return out, nil
}

// IssueCalendarToken — buat token feed ICS (gap#2: Google Calendar tidak bisa kirim header).
func (s *SolveService) IssueCalendarToken(scope, scopeID, label string) (*entity.CalendarToken, error) {
	if scope != "room" && scope != "group" && scope != "lecturer" {
		return nil, fmt.Errorf("%w: scope harus room|group|lecturer", ErrValidation)
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	t := &entity.CalendarToken{Token: hex.EncodeToString(b), Scope: scope, ScopeID: scopeID, Label: label}
	if err := s.db.Create(t).Error; err != nil {
		return nil, err
	}
	return t, nil
}

func (s *SolveService) ListCalendarTokens() ([]entity.CalendarToken, error) {
	var list []entity.CalendarToken
	return list, s.db.Order("created_at DESC").Limit(100).Find(&list).Error
}

func (s *SolveService) RevokeCalendarToken(id string) error {
	res := s.db.Model(&entity.CalendarToken{}).Where("id = ?", id).Update("revoked", true)
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ICSByToken — feed ICS publik via token (tanpa header auth).
func (s *SolveService) ICSByToken(token string) (string, string, error) {
	var t entity.CalendarToken
	if err := s.db.Where("token = ? AND revoked = ?", token, false).First(&t).Error; err != nil {
		return "", "", ErrNotFound
	}
	// term aktif
	var terms []entity.Term
	s.db.Where("is_active = ?", true).Order("start_date DESC").Find(&terms)
	if len(terms) == 0 {
		return "", "", ErrNoData
	}
	ics, err := s.BuildICS(terms[0].ID, t.Scope, t.ScopeID)
	if err != nil {
		return "", "", err
	}
	return ics, terms[0].ID, nil
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
