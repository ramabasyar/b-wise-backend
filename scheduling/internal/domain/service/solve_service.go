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
	"sort"
	"strconv"
	"strings"
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
// isGraduateProgram — true bila program pascasarjana (S2/S3/Magister/Doktor): boleh kelas Sabtu.
func isGraduateProgram(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "s2") || strings.Contains(n, "s3") ||
		strings.Contains(n, "magister") || strings.Contains(n, "doktor")
}

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
	// Ketersediaan dosen v2: semua mode di-load — logika agregasi di bawah
	// (whitelist available / blokir penuh / window jam → slot-level).
	var avails []entity.LecturerAvailability
	s.db.Find(&avails)

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
		RoomTypes:      make([]solverclient.SolverRoomType, 0, len(roomTypes)),
		Slots:          make([]solverclient.SolverSlot, 0, len(slots)),
		LecturerBlocks: make([]solverclient.SolverBlock, 0, len(avails))}
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
	// ---------- agregasi ketersediaan v2 ----------
	// 1) mode=available TANPA jam → whitelist hari: hari lain diblokir penuh
	//    (dosen magang "hanya bisa Rabu & Kamis" — tanpa input banyak blocker)
	// 2) mode=blocked TANPA jam → blokir hari penuh
	// 3) DENGAN jam: blocked → blokir slot yang menimpa window;
	//    available → blokir slot DI LUAR window hari itu (hanya window yang boleh)
	activeDays := map[int]bool{}
	for _, sl := range slots {
		activeDays[sl.Day] = true
	}
	whitelist := map[string]map[int]bool{}
	for _, a := range avails {
		if a.Mode == "available" && a.StartTime == "" && a.EndTime == "" {
			if whitelist[a.LecturerID] == nil {
				whitelist[a.LecturerID] = map[int]bool{}
			}
			whitelist[a.LecturerID][a.Day] = true
		}
	}
	blockedFull := map[string]map[int]bool{}
	blockedSlots := map[string]map[string]bool{}
	for _, a := range avails {
		if a.StartTime == "" && a.EndTime == "" {
			if a.Mode == "blocked" {
				if blockedFull[a.LecturerID] == nil {
					blockedFull[a.LecturerID] = map[int]bool{}
				}
				blockedFull[a.LecturerID][a.Day] = true
			}
			continue
		}
		if blockedSlots[a.LecturerID] == nil {
			blockedSlots[a.LecturerID] = map[string]bool{}
		}
		for _, sl := range slots {
			if sl.Day != a.Day {
				continue
			}
			overlap := sl.StartTime < a.EndTime && a.StartTime < sl.EndTime
			if (a.Mode == "blocked" && overlap) || (a.Mode == "available" && !overlap) {
				blockedSlots[a.LecturerID][sl.ID] = true
			}
		}
	}
	for lid, days := range whitelist {
		if blockedFull[lid] == nil {
			blockedFull[lid] = map[int]bool{}
		}
		for d := range activeDays {
			if !days[d] {
				blockedFull[lid][d] = true
			}
		}
	}
	for lid, days := range blockedFull {
		for d := range days {
			p.LecturerBlocks = append(p.LecturerBlocks, solverclient.SolverBlock{LecturerID: lid, Day: d})
		}
	}
	slotDay := map[string]int{}
	for _, sl := range slots {
		slotDay[sl.ID] = sl.Day
	}
	agg := map[string]map[int][]string{}
	for lid, sids := range blockedSlots {
		for sid := range sids {
			d := slotDay[sid]
			if agg[lid] == nil {
				agg[lid] = map[int][]string{}
			}
			agg[lid][d] = append(agg[lid][d], sid)
		}
	}
	for lid, byDay := range agg {
		for d, ids := range byDay {
			sort.Strings(ids)
			p.LecturerBlocks = append(p.LecturerBlocks, solverclient.SolverBlock{LecturerID: lid, Day: d, SlotIDs: ids})
		}
	}
	// ---------- kalender akademik → batasan pola mingguan (F3v2) ----------
	// CalendarEvent per tanggal, sedangkan solver menyusun POLA mingguan.
	// Hari grid diblokir hanya kalau libur/acara menutup hari itu di ≥ 50%
	// minggu efektif semester — gangguan 1-2 tanggal tetap urusan exception
	// per tanggal (Jadwal Harian), bukan batasan pola.
	const calBlockRatio = 0.5
	var term entity.Term
	if err := s.db.Where("id = ?", termID).First(&term).Error; err == nil &&
		!term.StartDate.IsZero() && !term.EndDate.IsZero() && !term.EndDate.Before(term.StartDate) {
		var calEvents []entity.CalendarEvent
		s.db.Where("term_id = ?", termID).Find(&calEvents)
		if len(calEvents) > 0 {
			// minggu efektif = minggu ISO unik dalam rentang semester
			weeks := map[string]bool{}
			for d := term.StartDate; !d.After(term.EndDate); d = d.AddDate(0, 0, 1) {
				y, w := d.ISOWeek()
				weeks[fmt.Sprintf("%04d-W%02d", y, w)] = true
			}
			// tanggal kalender unik per hari-grid (kolom DATE bisa terbaca
			// dengan komponen waktu → potong 10 char pertama)
			dateSeen := map[string]bool{}
			perDay := map[int]int{}
			for _, ev := range calEvents {
				ds := ev.Date
				if len(ds) > 10 {
					ds = ds[:10]
				}
				if dateSeen[ds] {
					continue
				}
				dateSeen[ds] = true
				t, err := time.Parse("2006-01-02", ds)
				if err != nil {
					continue
				}
				perDay[(int(t.Weekday())+6)%7+1]++ // 1=Senin..7=Minggu (format grid)
			}
			dayNames := []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu", "Minggu"}
			for d := 1; d <= 7; d++ {
				if !activeDays[d] || perDay[d] == 0 || len(weeks) == 0 {
					continue // hari tanpa slot di grid / tidak ada event
				}
				if float64(perDay[d])/float64(len(weeks)) >= calBlockRatio {
					p.CalendarBlocks = append(p.CalendarBlocks, solverclient.SolverCalendarBlock{
						Day:    d,
						Reason: fmt.Sprintf("%s: %d/%d minggu terganggu kalender", dayNames[d], perDay[d], len(weeks)),
					})
				}
			}
		}
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
			// F4-C: Sabtu (day 6) hanya utk kelas S2/Magister — sesi non-S2 diblokir.
			var bdays []int
			if o.ClassGroup != nil && !isGraduateProgram(o.ClassGroup.ProgramName) {
				bdays = []int{6}
			}
			p.Sessions = append(p.Sessions, solverclient.SolverSession{
				Key: fmt.Sprintf("%s#%d", o.ID, n), OfferingID: o.ID,
				CourseCode: ccode, CourseType: ctype, RoomNeed: cneed, RoomType: o.RoomType,
				GroupID: o.ClassGroupID, GroupSize: size, LecturerID: lect, LecturerIDs: cLects,
				DurationSlots: dur, DurationMinutes: theoryMin, BlockedDays: bdays,
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
			// F4-C: Sabtu hanya S2 — berlaku juga utk sesi praktikum non-S2.
			var bdays []int
			if o.ClassGroup != nil && !isGraduateProgram(o.ClassGroup.ProgramName) {
				bdays = []int{6}
			}
			practiceMin := o.PracticeSks * tp.SksMinutesPractice
			p.Sessions = append(p.Sessions, solverclient.SolverSession{
				Key: fmt.Sprintf("%s#p", o.ID), OfferingID: o.ID,
				CourseCode: ccode, CourseType: "practice", RoomNeed: "practice", RoomType: "",
				GroupID: o.ClassGroupID, GroupSize: size, LecturerID: lect, LecturerIDs: cLects,
				DurationSlots: (o.PracticeSks + 1) / 2, DurationMinutes: practiceMin, BlockedDays: bdays,
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

// ==================== Penyesuaian Jadwal — Lapis 1 (Move) ====================

var dayNames = []string{"", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

func hm2min(t string) int {
	p := strings.Split(t, ":")
	if len(p) != 2 {
		return -1
	}
	h, _ := strconv.Atoi(p[0])
	m, _ := strconv.Atoi(p[1])
	return h*60 + m
}

func min2hm(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// MoveEntry — geser 1 sesi draft ke slot/ruang baru dengan validasi keras.
// Durasi sesi dipertahankan; hasil move otomatis dikunci (locked) supaya
// re-solve berikutnya menghormati keputusan manual.
func (s *SolveService) MoveEntry(entryID, slotID, roomID string) (*entity.TimetableEntry, error) {
	var entry entity.TimetableEntry
	if err := s.db.First(&entry, "id = ?", entryID).Error; err != nil {
		return nil, errors.New("entri jadwal tidak ditemukan")
	}
	if entry.VersionID != nil {
		return nil, errors.New("entri milik versi terpublikasi — tidak dapat dipindah langsung")
	}
	var slot entity.TimeSlot
	if err := s.db.First(&slot, "id = ?", slotID).Error; err != nil {
		return nil, errors.New("slot waktu tidak ditemukan")
	}
	var room entity.Room
	if err := s.db.First(&room, "id = ?", roomID).Error; err != nil {
		return nil, errors.New("ruang tidak ditemukan")
	}
	ns, ne, err := s.validateMove(&entry, slot, room)
	if err != nil {
		return nil, err
	}
	day := slot.Day

	entry.SlotID = slot.ID
	entry.Day = day
	entry.SlotOrder = slot.Order
	entry.StartTime = min2hm(ns)
	entry.EndTime = min2hm(ne)
	entry.RoomID = room.ID
	entry.Locked = true
	if err := s.db.Save(&entry).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *SolveService) moveCheckRoomType(offeringID string, room entity.Room) error {
	var off entity.Offering
	if err := s.db.First(&off, "id = ?", offeringID).Error; err != nil {
		return nil // offering tak ketemu — jangan blokir move
	}
	need := off.RoomType
	if need == "" {
		var course entity.Course
		if err := s.db.First(&course, "id = ?", off.CourseID).Error; err == nil && course.Type != "" {
			var ct entity.CourseType
			if err := s.db.First(&ct, "code = ?", course.Type).Error; err == nil && ct.RoomNeed != "" {
				need = ct.RoomNeed
			}
		}
	}
	if need == "" || need == "any" || need == "none" {
		return nil
	}
	var rt entity.RoomType
	if err := s.db.First(&rt, "code = ?", room.Type).Error; err != nil {
		return nil
	}
	if need == "theory" && !rt.ForTheory {
		return errors.New("tipe ruang tidak cocok: sesi teori butuh ruang kelas teori")
	}
	if need == "practice" && !rt.ForPractice {
		return errors.New("tipe ruang tidak cocok: sesi praktikum butuh ruang praktik")
	}
	return nil
}

func (s *SolveService) moveCheckLecturerDay(lids []string, day int) error {
	if len(lids) == 0 || day < 1 || day > 6 {
		return nil
	}
	var avails []entity.LecturerAvailability
	s.db.Where("lecturer_id IN ?", lids).Find(&avails)
	allowed := map[string]map[int]bool{}
	hasAllowed := map[string]bool{}
	for _, a := range avails {
		if a.Mode == "available" && a.StartTime == "" && a.EndTime == "" {
			if allowed[a.LecturerID] == nil {
				allowed[a.LecturerID] = map[int]bool{}
				hasAllowed[a.LecturerID] = true
			}
			allowed[a.LecturerID][a.Day] = true
		}
	}
	for _, a := range avails {
		if a.Day != day {
			continue
		}
		if a.Mode == "blocked" && a.StartTime == "" && a.EndTime == "" {
			return fmt.Errorf("dosen diblokir di hari %s (ketersediaan dosen)", dayNames[day])
		}
		if hasAllowed[a.LecturerID] && !allowed[a.LecturerID][day] {
			return fmt.Errorf("dosen hanya bisa mengajar di hari tertentu — %s tidak termasuk (ketersediaan dosen)", dayNames[day])
		}
	}
	return nil
}

// validateMove — seluruh validasi pemindahan sesi (dipakai MoveEntry manual
// maupun engine penyesuaian otomatis Lapis 2). Return jam mulai/selesai baru.
func (s *SolveService) validateMove(entry *entity.TimetableEntry, slot entity.TimeSlot, room entity.Room) (int, int, error) {
	sd, ed := hm2min(entry.StartTime), hm2min(entry.EndTime)
	if sd < 0 || ed <= sd {
		return 0, 0, errors.New("waktu entri lama tidak valid")
	}
	ns := hm2min(slot.StartTime)
	ne := ns + (ed - sd)
	if ne > 21*60 {
		return 0, 0, fmt.Errorf("durasi sesi melewati batas hari (berakhir %s)", min2hm(ne))
	}
	day := slot.Day

	var clash []entity.TimetableEntry
	q := "version_id IS NULL AND day = ? AND start_time < ? AND end_time > ? AND id <> ?"
	s.db.Where(q+" AND room_id = ?", day, min2hm(ne), min2hm(ns), entry.ID, room.ID).Find(&clash)
	if len(clash) > 0 {
		return 0, 0, fmt.Errorf("bentrokan ruang: %s sudah memakai ruang ini (%s–%s)",
			clash[0].CourseCode, clash[0].StartTime, clash[0].EndTime)
	}
	if entry.GroupID != "" {
		s.db.Where(q+" AND group_id = ?", day, min2hm(ne), min2hm(ns), entry.ID, entry.GroupID).Find(&clash)
		if len(clash) > 0 {
			return 0, 0, fmt.Errorf("bentrokan rombel: sudah ada sesi %s pada jam %s–%s",
				clash[0].CourseCode, clash[0].StartTime, clash[0].EndTime)
		}
	}
	var lids []string
	s.db.Model(&entity.OfferingLecturer{}).Where("offering_id = ?", entry.OfferingID).Pluck("lecturer_id", &lids)
	if len(lids) == 0 && entry.LecturerID != "" {
		lids = []string{entry.LecturerID}
	}
	if len(lids) > 0 {
		var otherOffs []string
		s.db.Model(&entity.OfferingLecturer{}).
			Where("lecturer_id IN ? AND offering_id <> ?", lids, entry.OfferingID).
			Distinct("offering_id").Pluck("offering_id", &otherOffs)
		if len(otherOffs) > 0 {
			s.db.Where(q+" AND offering_id IN ?", day, min2hm(ne), min2hm(ns), entry.ID, otherOffs).Find(&clash)
			if len(clash) > 0 {
				return 0, 0, fmt.Errorf("bentrokan dosen: pengampu sudah mengajar %s (%s–%s)",
					clash[0].CourseCode, clash[0].StartTime, clash[0].EndTime)
			}
		}
	}
	if entry.GroupID != "" && room.Capacity > 0 {
		var grp entity.ClassGroup
		if err := s.db.First(&grp, "id = ?", entry.GroupID).Error; err == nil && grp.SizeEst > room.Capacity {
			return 0, 0, fmt.Errorf("kapasitas tidak cukup: rombel ~%d mhs > ruang %d kursi", grp.SizeEst, room.Capacity)
		}
	}
	if err := s.moveCheckRoomType(entry.OfferingID, room); err != nil {
		return 0, 0, err
	}
	if err := s.moveCheckLecturerDay(lids, day); err != nil {
		return 0, 0, err
	}
	return ns, ne, nil
}

// ==================== Penyesuaian Jadwal — Lapis 2 (Proposal Otomatis) ====================

// AdjChange — satu baris diff perubahan.
type AdjChange struct {
	EntryID    string `json:"entry_id"`
	CourseCode string `json:"course_code"`
	GroupCode  string `json:"group_code"`
	FromDay    int    `json:"from_day"`
	FromJam    string `json:"from_jam"`
	FromRoom   string `json:"from_room"`
	ToDay      int    `json:"to_day"`
	ToJam      string `json:"to_jam"`
	ToRoom     string `json:"to_room"`
	ToSlotID   string `json:"to_slot_id"`
	ToRoomID   string `json:"to_room_id"`
}

// AdjUnresolved — sesi terdampak yang tidak mendapat pengganti valid.
type AdjUnresolved struct {
	EntryID    string `json:"entry_id"`
	CourseCode string `json:"course_code"`
	Reason     string `json:"reason"`
}

func absI(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// date10 — normalisasi string tanggal: kolom DATE terbaca GORM dgn komponen waktu.
func date10(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// ProposeAdjustment — deteksi sesi terdampak dari ketersediaan ruang (rentang tanggal
// → hari kuliah, jam overlap) lalu susun usulan pemindahan per sesi dengan validasi
// penuh. Semua sesi lain dianggap tetap. Proposal disimpan PENDING menunggu keputusan.
func (s *SolveService) ProposeAdjustment(roomAvailID, actorID string) (*entity.AdjustmentProposal, error) {
	var av entity.RoomAvailability
	if err := s.db.First(&av, "id = ?", roomAvailID).Error; err != nil {
		return nil, errors.New("ketersediaan ruang tidak ditemukan")
	}
	if !av.IsActive {
		return nil, errors.New("ketersediaan ruang tidak aktif")
	}
	// kolom date terbaca balik dgn komponen waktu (mis. "2026-09-21T00:00:00Z") — normalisasi
	normDate := func(s string) (time.Time, error) {
		if len(s) > 10 {
			s = s[:10]
		}
		return time.Parse("2006-01-02", s)
	}
	d1, e1 := normDate(av.StartDate)
	d2, e2 := normDate(av.EndDate)
	if e1 != nil || e2 != nil || d2.Before(d1) {
		return nil, errors.New("rentang tanggal tidak valid")
	}
	daySet := map[int]bool{}
	for d := d1; !d.After(d2); d = d.AddDate(0, 0, 1) {
		wd := int(d.Weekday())
		if wd == 0 {
			wd = 7
		}
		if wd >= 1 && wd <= 6 {
			daySet[wd] = true
		}
	}
	dayList := []int{}
	for d := 1; d <= 6; d++ {
		if daySet[d] {
			dayList = append(dayList, d)
		}
	}
	if len(dayList) == 0 {
		return nil, errors.New("rentang tanggal tidak menyentuh hari kuliah")
	}
	q := s.db.Where("version_id IS NULL AND room_id = ? AND day IN ?", av.RoomID, dayList)
	if av.StartTime != "" && av.EndTime != "" {
		q = q.Where("start_time < ? AND end_time > ?", av.EndTime, av.StartTime)
	}
	var hit []entity.TimetableEntry
	q.Order("day, start_time").Find(&hit)
	if len(hit) == 0 {
		return nil, errors.New("tidak ada sesi terdampak untuk gangguan ini")
	}
	term := hit[0].TermID

	var rooms []entity.Room
	s.db.Where("is_active = ?", true).Order("capacity ASC").Find(&rooms)
	var slots []entity.TimeSlot
	s.db.Order("day, \"order\"").Find(&slots)
	sizeOf, codeOfGroup, codeOfRoom := map[string]int{}, map[string]string{}, map[string]string{}
	{
		var grps []entity.ClassGroup
		s.db.Find(&grps)
		for _, g := range grps {
			sizeOf[g.ID] = g.SizeEst
			codeOfGroup[g.ID] = g.Code
		}
		for _, r := range rooms {
			codeOfRoom[r.ID] = r.Code
		}
	}

	changes := []AdjChange{}
	unres := []AdjUnresolved{}
	virtual := []entity.TimetableEntry{} // okupansi rencana supaya antar-usulan tidak saling menabrak
	for _, e := range hit {
		if e.Locked {
			unres = append(unres, AdjUnresolved{EntryID: e.ID, CourseCode: e.CourseCode, Reason: "terkunci manual — tidak digeser otomatis"})
			continue
		}
		sortedSlots := make([]entity.TimeSlot, len(slots))
		copy(sortedSlots, slots)
		es := e
		sort.SliceStable(sortedSlots, func(a, b int) bool {
			sa, sb := sortedSlots[a], sortedSlots[b]
			da, db := absI(sa.Day-es.Day), absI(sb.Day-es.Day)
			if da != db {
				return da < db
			}
			return absI(sa.Order-es.SlotOrder) < absI(sb.Order-es.SlotOrder)
		})
		sortedRooms := make([]entity.Room, len(rooms))
		copy(sortedRooms, rooms)
		sz := sizeOf[e.GroupID]
		fit := func(c int) int {
			if c > 0 && c < sz {
				return 1 << 30
			}
			return absI(c - sz)
		}
		sort.SliceStable(sortedRooms, func(a, b int) bool {
			return fit(sortedRooms[a].Capacity) < fit(sortedRooms[b].Capacity)
		})
		dur := hm2min(e.EndTime) - hm2min(e.StartTime)
		found := false
		for _, sl := range sortedSlots {
			for _, r := range sortedRooms {
				if r.ID == av.RoomID {
					continue
				}
				ns, _, verr := s.validateMove(&es, sl, r)
				if verr != nil {
					continue
				}
				ne := ns + dur
				bump := false
				for _, v := range virtual {
					if v.Day == sl.Day && v.RoomID == r.ID && hm2min(v.StartTime) < ne && ns < hm2min(v.EndTime) {
						bump = true
						break
					}
				}
				if bump {
					continue
				}
				changes = append(changes, AdjChange{
					EntryID: e.ID, CourseCode: e.CourseCode, GroupCode: codeOfGroup[e.GroupID],
					FromDay: e.Day, FromJam: e.StartTime + "–" + e.EndTime, FromRoom: codeOfRoom[av.RoomID],
					ToDay: sl.Day, ToJam: min2hm(ns) + "–" + min2hm(ne), ToRoom: r.Code,
					ToSlotID: sl.ID, ToRoomID: r.ID,
				})
				nv := e
				nv.Day, nv.SlotID, nv.RoomID = sl.Day, sl.ID, r.ID
				nv.StartTime, nv.EndTime = min2hm(ns), min2hm(ne)
				virtual = append(virtual, nv)
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			unres = append(unres, AdjUnresolved{EntryID: e.ID, CourseCode: e.CourseCode, Reason: "tidak menemukan slot/ruang pengganti yang valid"})
		}
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("tidak ada usulan pemindahan valid: %d sesi terdampak, %d tak teratasi", len(hit), len(unres))
	}
	chJ, _ := json.Marshal(changes)
	unJ, _ := json.Marshal(unres)
	p := &entity.AdjustmentProposal{
		TermID: term, RoomAvailabilityID: av.ID,
		Reason: fmt.Sprintf("Ruang %s: %s", codeOfRoom[av.RoomID], av.Reason),
		Status: "pending", Changes: string(chJ), Unresolved: string(unJ), CreatedBy: actorID,
	}
	if err := s.db.Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// ListAdjustments — daftar proposal (default pending).
// ProposeManualAdjustment — usulan pemindahan MANUAL satu sesi oleh user
// (dosen berhalangan, permintaan prodi, dsb) → proposal PENDING menunggu
// keputusan (DecideAdjustment yang sama menerapkan: pindah + terkunci).
// Validasi penuh via validateMove; konflik → ditolak dgn pesan agar user pilih slot/ruang lain.
func (s *SolveService) ProposeManualAdjustment(entryID, toSlotID, toRoomID, reason, actorID string) (*entity.AdjustmentProposal, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("alasan wajib diisi")
	}
	var entry entity.TimetableEntry
	if err := s.db.First(&entry, "id = ?", entryID).Error; err != nil {
		return nil, errors.New("entri jadwal tidak ditemukan")
	}
	if entry.VersionID != nil {
		return nil, errors.New("entri milik versi terpublikasi — tidak dapat diajukan")
	}
	var slot entity.TimeSlot
	if err := s.db.First(&slot, "id = ?", toSlotID).Error; err != nil {
		return nil, errors.New("slot waktu tujuan tidak ditemukan")
	}
	roomID := toRoomID
	if roomID == "" {
		roomID = entry.RoomID // kosong = ruang tetap
	}
	var room entity.Room
	if err := s.db.First(&room, "id = ?", roomID).Error; err != nil {
		return nil, errors.New("ruang tujuan tidak ditemukan")
	}
	ns, ne, err := s.validateMove(&entry, slot, room)
	if err != nil {
		return nil, fmt.Errorf("tidak bisa diajukan: %s", err.Error())
	}
	fromRoom := ""
	if entry.RoomID != "" {
		var fromR entity.Room
		if err := s.db.First(&fromR, "id = ?", entry.RoomID).Error; err == nil {
			fromRoom = fromR.Code
		}
	}
	groupCode := ""
	var grp entity.ClassGroup
	if err := s.db.First(&grp, "id = ?", entry.GroupID).Error; err == nil {
		groupCode = grp.Code
	}
	ch := AdjChange{
		EntryID: entry.ID, CourseCode: entry.CourseCode, GroupCode: groupCode,
		FromDay: entry.Day, FromJam: entry.StartTime + "–" + entry.EndTime, FromRoom: fromRoom,
		ToDay: slot.Day, ToJam: min2hm(ns) + "–" + min2hm(ne), ToRoom: room.Code,
		ToSlotID: slot.ID, ToRoomID: room.ID,
	}
	chJSON, _ := json.Marshal([]AdjChange{ch})
	p := &entity.AdjustmentProposal{
		TermID: entry.TermID, Reason: reason,
		Changes: string(chJSON), Unresolved: "[]", CreatedBy: actorID,
	}
	if err := s.db.Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// EntryRow — baris ringkas utk form penyesuaian manual (portal).
type EntryRow struct {
	ID         string `json:"id"`
	CourseCode string `json:"course_code"`
	GroupCode  string `json:"group_code"`
	Day        int    `json:"day"`
	Jam        string `json:"jam"`
	RoomCode   string `json:"room_code"`
	RoomID     string `json:"room_id"`
	SlotID     string `json:"slot_id"`
}

// ListEntriesForAdjustment — daftar entri DRAFT (flat) utk pilih sesi di form penyesuaian manual.
func (s *SolveService) ListEntriesForAdjustment(termID string) ([]EntryRow, error) {
	rows := []EntryRow{}
	q := `SELECT te.id, te.course_code, COALESCE(cg.code,'') AS group_code, te.day,
	      te.start_time||'–'||te.end_time AS jam, COALESCE(r.code,'') AS room_code,
	      te.room_id, te.slot_id
	      FROM timetable_entries te
	      LEFT JOIN class_groups cg ON cg.id = te.group_id
	      LEFT JOIN rooms r ON r.id = te.room_id
	      WHERE te.version_id IS NULL AND (? = '' OR te.term_id = ?)
	      ORDER BY te.day, te.start_time, te.course_code`
	if err := s.db.Raw(q, termID, termID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *SolveService) ListAdjustments(status string) ([]entity.AdjustmentProposal, error) {
	var items []entity.AdjustmentProposal
	if err := s.db.Where("status = ?", status).Order("created_at DESC").Limit(50).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// DecideAdjustment — setujui (terapkan diff: entri pindah + terkunci) atau tolak.
func (s *SolveService) DecideAdjustment(id string, approve bool, actorID string) (*entity.AdjustmentProposal, error) {
	var p entity.AdjustmentProposal
	if err := s.db.First(&p, "id = ?", id).Error; err != nil {
		return nil, errors.New("proposal tidak ditemukan")
	}
	if p.Status != "pending" {
		return nil, errors.New("proposal sudah diputuskan")
	}
	if approve {
		var changes []AdjChange
		if err := json.Unmarshal([]byte(p.Changes), &changes); err != nil {
			return nil, err
		}
		var slots []entity.TimeSlot
		s.db.Find(&slots)
		slotByID := map[string]entity.TimeSlot{}
		for _, sl := range slots {
			slotByID[sl.ID] = sl
		}
		// Pintar: gangguan SATU TANGGAL → exception per tanggal (pola mingguan TIDAK
		// berubah — minggu berikutnya kembali normal). Rentang → move permanen pola.
		var av entity.RoomAvailability
		hasAv := s.db.First(&av, "id = ?", p.RoomAvailabilityID).Error == nil
		singleDate := hasAv && date10(av.StartDate) == date10(av.EndDate)
		for _, ch := range changes {
			var e entity.TimetableEntry
			if err := s.db.First(&e, "id = ?", ch.EntryID).Error; err != nil {
				continue
			}
			if singleDate {
				s.db.Where("date = ? AND entry_id = ?", date10(av.StartDate), ch.EntryID).
					Delete(&entity.EntryOverride{})
				ov := entity.EntryOverride{
					TermID: p.TermID, Date: date10(av.StartDate), EntryID: ch.EntryID,
					Kind: "moved", SlotID: ch.ToSlotID, RoomID: ch.ToRoomID,
					Source: "adjustment:" + p.ID, Reason: p.Reason,
				}
				s.db.Create(&ov)
				continue
			}
			sl, ok := slotByID[ch.ToSlotID]
			if !ok {
				continue
			}
			ns := hm2min(sl.StartTime)
			dur := hm2min(e.EndTime) - hm2min(e.StartTime)
			e.SlotID, e.Day, e.SlotOrder = sl.ID, sl.Day, sl.Order
			e.StartTime, e.EndTime = min2hm(ns), min2hm(ns+dur)
			e.RoomID, e.Locked = ch.ToRoomID, true
			s.db.Save(&e)
		}
		p.Status = "applied"
	} else {
		p.Status = "rejected"
	}
	p.DecidedBy = actorID
	s.db.Save(&p)
	return &p, nil
}

// ==================== Penyesuaian Jadwal — Fase 3: Exception per Tanggal ====================

// DaySession — satu baris jadwal efektif pada sebuah tanggal.
type DaySession struct {
	EntryID    string `json:"entry_id"`
	CourseCode string `json:"course_code"`
	CourseName string `json:"course_name"`
	GroupCode  string `json:"group_code"`
	Lecturer   string `json:"lecturer"`
	Jam        string `json:"jam"`
	RoomCode   string `json:"room_code"`
	Status     string `json:"status"` // normal|moved|cancelled
	MovedFrom  string `json:"moved_from,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// DayView — jadwal efektif sebuah tanggal kalender: pola mingguan (published ?? draft)
// + entry_overrides diterapkan + calendar_events sebagai banner.
func (s *SolveService) DayView(date, termID string) (map[string]any, error) {
	d, err := time.Parse("2006-01-02", date10(date))
	if err != nil {
		return nil, errors.New("format tanggal harus YYYY-MM-DD")
	}
	wd := int(d.Weekday())
	res := map[string]any{"date": date10(date), "weekday": wd, "sessions": []DaySession{}}
	if wd == 0 {
		res["note"] = "Hari Minggu — tidak ada perkuliahan"
		return res, nil
	}
	var cals []entity.CalendarEvent
	s.db.Where("date = ?", date10(date)).Find(&cals)
	if len(cals) > 0 {
		res["event"] = map[string]string{"name": cals[0].Name, "kind": cals[0].Kind}
	}
	// pilih term: parameter, atau term dgn entries terbanyak di hari itu
	tid := termID
	if tid == "" {
		var row struct{ TermID string }
		s.db.Raw("SELECT term_id FROM timetable_entries WHERE day = ? AND version_id IS NOT NULL GROUP BY term_id ORDER BY count(*) DESC LIMIT 1", wd).Scan(&row)
		if row.TermID == "" {
			s.db.Raw("SELECT term_id FROM timetable_entries WHERE day = ? GROUP BY term_id ORDER BY count(*) DESC LIMIT 1", wd).Scan(&row)
		}
		tid = row.TermID
	}
	if tid == "" {
		res["note"] = "Belum ada jadwal untuk hari ini"
		return res, nil
	}
	res["term_id"] = tid
	// entries: published lebih diutamakan; fallback draft
	var entries []entity.TimetableEntry
	s.db.Preload("Room").Preload("ClassGroup").
		Where("term_id = ? AND day = ? AND version_id IS NOT NULL", tid, wd).
		Order("start_time").Find(&entries)
	if len(entries) == 0 {
		s.db.Preload("Room").Preload("ClassGroup").
			Where("term_id = ? AND day = ? AND version_id IS NULL", tid, wd).
			Order("start_time").Find(&entries)
		res["source"] = "draft"
	} else {
		res["source"] = "published"
	}
	// overrides + jam override — resolve via session_key supaya berlaku lintas versi
	// (engine L2 bekerja di draft; day-view menampilkan published ?? draft)
	var ovs []entity.EntryOverride
	s.db.Where("date = ? AND term_id = ?", date10(date), tid).Find(&ovs)
	ovBy := map[string]entity.EntryOverride{}
	for _, o := range ovs {
		var src entity.TimetableEntry
		if s.db.First(&src, "id = ?", o.EntryID).Error == nil && src.SessionKey != "" {
			ovBy[src.SessionKey] = o
		} else {
			ovBy[o.EntryID] = o
		}
	}
	var slots []entity.TimeSlot
	s.db.Find(&slots)
	slotByID := map[string]entity.TimeSlot{}
	for _, sl := range slots {
		slotByID[sl.ID] = sl
	}
	var rooms []entity.Room
	s.db.Find(&rooms)
	roomCode := map[string]string{}
	for _, r := range rooms {
		roomCode[r.ID] = r.Code
	}
	// nama MK + dosen pengampu utama
	type offInfo struct{ course, lect string }
	offMap := map[string]offInfo{}
	{
		var offs []entity.Offering
		s.db.Preload("Course").Preload("Lecturer").Find(&offs)
		for _, o := range offs {
			cn, ln := "", ""
			if o.Course != nil {
				cn = o.Course.Name
			}
			if o.Lecturer != nil {
				ln = o.Lecturer.Name
			}
			offMap[o.ID] = offInfo{cn, ln}
		}
	}
	out := []DaySession{}
	for _, e := range entries {
		ds := DaySession{
			EntryID: e.ID, CourseCode: e.CourseCode,
			CourseName: offMap[e.OfferingID].course,
			GroupCode:  "", Lecturer: offMap[e.OfferingID].lect,
			Jam:      e.StartTime + "–" + e.EndTime,
			RoomCode: "", Status: "normal",
		}
		if e.Room != nil {
			ds.RoomCode = e.Room.Code
		}
		if e.ClassGroup != nil {
			ds.GroupCode = e.ClassGroup.Code
		}
		if ov, ok := ovBy[e.SessionKey]; ok {
			// (juga cek by-ID utk override manual pada entri yg ditampilkan)
			ds.Reason = ov.Reason
			if ov.Kind == "cancelled" {
				ds.Status = "cancelled"
			} else if sl, ok2 := slotByID[ov.SlotID]; ok2 {
				ns := hm2min(sl.StartTime)
				dur := hm2min(e.EndTime) - hm2min(e.StartTime)
				ds.Status = "moved"
				ds.MovedFrom = fmt.Sprintf("asli %s @%s", ds.Jam, ds.RoomCode)
				ds.Jam = min2hm(ns) + "–" + min2hm(ns+dur)
				ds.RoomCode = roomCode[ov.RoomID]
			}
		}
		out = append(out, ds)
	}
	res["sessions"] = out
	return res, nil
}

// CreateOverride — pengecualian manual (moved/cancelled) untuk satu tanggal.
func (s *SolveService) CreateOverride(termID, date, entryID, kind, slotID, roomID, reason string) (*entity.EntryOverride, error) {
	if _, err := time.Parse("2006-01-02", date10(date)); err != nil {
		return nil, errors.New("format tanggal harus YYYY-MM-DD")
	}
	var e entity.TimetableEntry
	if err := s.db.First(&e, "id = ?", entryID).Error; err != nil {
		return nil, errors.New("sesi tidak ditemukan")
	}
	if kind != "moved" && kind != "cancelled" {
		return nil, errors.New("kind harus moved atau cancelled")
	}
	if kind == "moved" {
		var sl entity.TimeSlot
		if err := s.db.First(&sl, "id = ?", slotID).Error; err != nil {
			return nil, errors.New("slot waktu tidak ditemukan")
		}
		var r entity.Room
		if err := s.db.First(&r, "id = ?", roomID).Error; err != nil {
			return nil, errors.New("ruang tidak ditemukan")
		}
	}
	if termID == "" {
		termID = e.TermID
	}
	// satu override per (tanggal, sesi) — replace
	s.db.Where("date = ? AND entry_id = ?", date10(date), entryID).Delete(&entity.EntryOverride{})
	ov := entity.EntryOverride{
		TermID: termID, Date: date10(date), EntryID: entryID, Kind: kind,
		SlotID: slotID, RoomID: roomID, Source: "manual", Reason: reason,
	}
	if err := s.db.Create(&ov).Error; err != nil {
		return nil, err
	}
	return &ov, nil
}

// DeleteOverride — hapus pengecualian (kembali ke pola normal).
func (s *SolveService) DeleteOverride(id string) error {
	if err := s.db.Delete(&entity.EntryOverride{}, "id = ?", id).Error; err != nil {
		return err
	}
	return nil
}
