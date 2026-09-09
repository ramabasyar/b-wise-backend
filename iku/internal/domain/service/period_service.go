package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	entity "github.com/rama/b-wise/iku/internal/domain/entity"
	"gorm.io/gorm"
)

// ==================== PERIOD LIFECYCLE SERVICE (F9) ====================
// Keputusan design (disetujui Mas Rama, 19 Aug 2026):
//  1. Buka periode  : MANUAL oleh admin (periods.write) — provisioning otomatis, aktivasi manusia.
//  2. Tutup periode : HYBRID — manual oleh reviewer BPM (achievements.workflow) / admin,
//                     ATAU auto-close oleh sistem setelah grace (default 14 hari) lewat.
//  3. Sisa draft/submitted saat tutup : DIBEKUKAN status "missed" (tercatat, akuntabel).
//  4. Rollup tahunan: OTOMATIS saat 4 TW tertutup — nilai per indikator diagregasi
//     (sum|avg|last dari IndicatorDefinition.RollupRule, default avg) jadi capaian
//     tahunan DRAFT menunggu review. Reopen TW pasca-rollup -> tahunan ditandai STALE.
//
// Status periode: provisioned -> open -> grace -> closed.  (closed -> open = reopen, alasan wajib)

const DefaultGraceDays = 14

type PeriodService struct {
	db           *gorm.DB
	notifier     *Notifier
	graceNoticed map[string]string // periodID -> tanggal (YYYY-MM-DD) notifikasi harian terakhir
}

func NewPeriodService(db *gorm.DB, notifier *Notifier) *PeriodService {
	return &PeriodService{db: db, notifier: notifier, graceNoticed: map[string]string{}}
}

// EnsureYearPeriods — provision TW1..4 + tahunan untuk `year` (idempoten by label).
// Record baru berstatus "provisioned"; record lama tidak pernah disentuh.
func EnsureYearPeriods(db *gorm.DB, year int) error {
	for q := 1; q <= 4; q++ {
		label := fmt.Sprintf("TW%d-%d", q, year)
		var count int64
		db.Model(&entity.Period{}).Where("label = ?", label).Count(&count)
		if count == 0 {
			start := time.Date(year, time.Month(3*q-2), 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 3, 0).AddDate(0, 0, -1)
			due := end
			if err := db.Create(&entity.Period{
				Label: label, Type: "quarterly", Year: year, Sequence: q,
				StartDate: start, EndDate: end, DueDate: &due,
				GraceDays: DefaultGraceDays, Status: "provisioned",
			}).Error; err != nil {
				return err
			}
		}
	}
	annLabel := fmt.Sprintf("%d", year)
	var countAnn int64
	db.Model(&entity.Period{}).Where("label = ?", annLabel).Count(&countAnn)
	if countAnn == 0 {
		start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		end := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
		due := end
		if err := db.Create(&entity.Period{
			Label: annLabel, Type: "annual", Year: year,
			StartDate: start, EndDate: end, DueDate: &due,
			GraceDays: DefaultGraceDays, Status: "provisioned",
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// List — periode terurut tahun terbaru dulu.
func (s *PeriodService) List(year int, ptype string) ([]entity.Period, error) {
	q := s.db.Model(&entity.Period{})
	if year > 0 {
		q = q.Where("year = ?", year)
	}
	if ptype != "" {
		q = q.Where("type = ?", ptype)
	}
	var list []entity.Period
	return list, q.Order("year DESC, type ASC, sequence ASC").Find(&list).Error
}

// CountStats — jumlah capaian per (periodID -> status -> n) untuk badge UI.
func (s *PeriodService) CountStats() (map[string]map[string]int, error) {
	rows := []struct {
		PeriodID string
		Status   string
		N        int
	}{}
	if err := s.db.Model(&entity.AchievementRecord{}).
		Select("period_id, status, count(*) as n").
		Where("deleted_at IS NULL").
		Group("period_id, status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]map[string]int{}
	for _, r := range rows {
		if out[r.PeriodID] == nil {
			out[r.PeriodID] = map[string]int{}
		}
		out[r.PeriodID][r.Status] = r.N
	}
	return out, nil
}

// Get — detail satu periode + counts.
func (s *PeriodService) Get(id string) (*entity.Period, map[string]int, error) {
	var p entity.Period
	if err := s.db.Where("id = ?", id).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}
	stats, _ := s.CountStats()
	return &p, stats[p.ID], nil
}

// OpenPeriod — provisioned->open (buka) atau closed->open (reopen, alasan WAJIB).
func (s *PeriodService) OpenPeriod(id, actor, note string) (*entity.Period, error) {
	var p entity.Period
	if err := s.db.Where("id = ?", id).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if p.Status != "provisioned" && p.Status != "closed" {
		return nil, fmt.Errorf("%w: periode status %s tidak bisa dibuka", ErrValidation, p.Status)
	}
	isReopen := p.Status == "closed"
	if isReopen && strings.TrimSpace(note) == "" {
		return nil, fmt.Errorf("%w: reopen wajib menyertakan alasan (tercatat di audit)", ErrValidation)
	}
	now := time.Now()
	if err := s.db.Model(&entity.Period{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status": "open", "opened_by": actor, "opened_at": now,
	}).Error; err != nil {
		return nil, err
	}
	// Reopen TW pasca-rollup -> tahunan jadi stale (nilai final tidak pernah berubah diam-diam)
	if isReopen && p.Type == "quarterly" {
		if n, err := s.markRollupStale(p.Year); err == nil && n > 0 && s.notifier != nil {
			s.notifier.NotifyAdmins("period",
				fmt.Sprintf("Rollup tahunan %d kedaluwarsa", p.Year),
				fmt.Sprintf("TW di-reopen setelah rollup digenerate (%d capaian tahunan ditandai stale). Generate ulang setelah TW kembali ditutup.", n),
				"period", p.ID)
		}
	}
	s.db.Where("id = ?", id).First(&p)
	if s.notifier != nil {
		action := "dibuka"
		if isReopen {
			action = "di-reopen"
		}
		s.notifier.NotifyAdmins("period", fmt.Sprintf("Periode %s %s", p.Label, action),
			fmt.Sprintf("Oleh %s. %s", actor, note), "period", p.ID)
	}
	return &p, nil
}

// CloseSummary — hasil operasi tutup periode.
type CloseSummary struct {
	Period   entity.Period `json:"period"`
	Missed   int           `json:"missed"`    // capaian dibekukan (draft/submitted -> missed)
	RolledUp int           `json:"rolled_up"` // capaian tahunan digenerate (jika 4 TW tertutup)
}

// ClosePeriod — open|grace -> closed. Bekukan draft/submitted jadi missed. TX atomik.
func (s *PeriodService) ClosePeriod(id, actor, note string, auto bool) (*CloseSummary, error) {
	var p entity.Period
	if err := s.db.Where("id = ?", id).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if p.Status != "open" && p.Status != "grace" {
		return nil, fmt.Errorf("%w: hanya periode open/grace yang bisa ditutup (status: %s)", ErrValidation, p.Status)
	}
	by := actor
	if auto {
		by = "system:auto-close"
	}
	summary := &CloseSummary{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&entity.Period{}).Where("id = ?", id).Updates(map[string]interface{}{
			"status": "closed", "closed_by": by, "closed_at": now, "close_note": note,
		}).Error; err != nil {
			return err
		}
		// Bekukan sisa draft/submitted -> missed (keputusan #3: dibekukan, tercatat)
		var ids []string
		if err := tx.Model(&entity.AchievementRecord{}).
			Where("period_id = ? AND status IN ? AND deleted_at IS NULL", id, []string{"draft", "submitted"}).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		for _, aid := range ids {
			if err := tx.Model(&entity.AchievementRecord{}).Where("id = ?", aid).Update("status", "missed").Error; err != nil {
				return err
			}
			tx.Create(&entity.WorkflowLog{
				AchievementID: aid, Action: "missed", ActorID: by,
				Notes: fmt.Sprintf("periode %s ditutup — capaian dibekukan (tidak selesai submit/review)", p.Label),
			})
		}
		summary.Missed = len(ids)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.db.Where("id = ?", id).First(&summary.Period)
	if s.notifier != nil {
		s.notifier.NotifyAdmins("period", fmt.Sprintf("Periode %s ditutup", p.Label),
			fmt.Sprintf("Ditutup oleh %s. %d capaian dibekukan (missed). %s", by, summary.Missed, note), "period", p.ID)
	}
	// Rollup tahunan otomatis (keputusan #4) — hanya saat triwulan yang ditutup
	if p.Type == "quarterly" {
		if n, err := s.TryRollup(p.Year); err == nil {
			summary.RolledUp = n
		} else {
			log.Printf("[period] rollup %d gagal: %v", p.Year, err)
		}
	}
	return summary, nil
}

// markRollupStale — tahunan hasil rollup ditandai kedaluwarsa (TW di-reopen).
func (s *PeriodService) markRollupStale(year int) (int, error) {
	var ann entity.Period
	if err := s.db.Where("year = ? AND type = 'annual'", year).First(&ann).Error; err != nil {
		return 0, nil // tahunan belum ada — tidak ada yang perlu stale
	}
	res := s.db.Model(&entity.AchievementRecord{}).
		Where("period_id = ? AND source = ? AND stale = false", ann.ID, "rollup").
		Update("stale", true)
	return int(res.RowsAffected), res.Error
}

// TryRollup — generate capaian TAHUNAN (draft) dari 4 triwulan yang sudah closed.
// Syarat: 4 TW tahun `year` semua closed + periode annual ada.
// Per pasangan (indikator, unit): nilai = sum|avg|last sesuai indikator.RollupRule,
// dari capaian TW berstatus final (reviewed|approved|published). Input manual tahunan
// MENANG (tidak ditimpa rollup). Rollup yang sudah approved/published tidak diubah
// nilainya — hanya ditandai stale bila sumber TW berubah.
func (s *PeriodService) TryRollup(year int) (int, error) {
	// hanya TW kanonik 1-4 (abaikan periode quarterly khusus tambahan)
	var tws []entity.Period
	if err := s.db.Where("year = ? AND type = 'quarterly' AND sequence BETWEEN 1 AND 4", year).
		Order("sequence ASC").Find(&tws).Error; err != nil {
		return 0, err
	}
	if len(tws) < 4 {
		return 0, nil
	}
	for _, tw := range tws {
		if tw.Status != "closed" {
			return 0, nil // belum semua tertutup
		}
	}
	var ann entity.Period
	if err := s.db.Where("year = ? AND type = 'annual'", year).First(&ann).Error; err != nil {
		return 0, nil // periode tahunan belum ada
	}

	twIDs := make([]string, len(tws))
	seqOf := map[string]int{}
	for i, tw := range tws {
		twIDs[i] = tw.ID
		seqOf[tw.ID] = tw.Sequence
	}

	var rows []entity.AchievementRecord
	if err := s.db.Where("period_id IN ? AND status IN ? AND deleted_at IS NULL",
		twIDs, []string{"reviewed", "approved", "published"}).Find(&rows).Error; err != nil {
		return 0, err
	}

	type key struct{ ind, unit, utype string }
	grouped := map[key][]entity.AchievementRecord{}
	for _, r := range rows {
		k := key{r.IndicatorID, r.UnitID, r.UnitType}
		grouped[k] = append(grouped[k], r)
	}

	created := 0
	for k, recs := range grouped {
		// input manual tahunan menang — skip pasangan yang sudah diinput manual
		var manual int64
		s.db.Model(&entity.AchievementRecord{}).
			Where("period_id = ? AND indicator_id = ? AND unit_id = ? AND source <> 'rollup' AND deleted_at IS NULL",
				ann.ID, k.ind, k.unit).Count(&manual)
		if manual > 0 {
			continue
		}
		var ind entity.IndicatorDefinition
		if err := s.db.Where("id = ?", k.ind).First(&ind).Error; err != nil {
			continue
		}
		rule := ind.RollupRule
		if rule != "sum" && rule != "last" {
			rule = "avg"
		}
		val := aggregateRollup(recs, seqOf, rule)

		var ex entity.AchievementRecord
		exErr := s.db.Where("period_id = ? AND indicator_id = ? AND unit_id = ? AND deleted_at IS NULL",
			ann.ID, k.ind, k.unit).First(&ex).Error
		if exErr == nil {
			// sudah final -> jangan ubah nilai; tandai stale supaya admin regenerate eksplisit
			if ex.Status == "approved" || ex.Status == "published" {
				if !ex.Stale {
					s.db.Model(&ex).Update("stale", true)
				}
				continue
			}
			ex.CalculatedValue = val
			ex.Status = "draft"
			ex.Stale = false
			ex.Source = "rollup"
			s.applyRollupTarget(&ex, &ind)
			s.db.Save(&ex)
			s.db.Create(&entity.WorkflowLog{AchievementID: ex.ID, Action: "rollup", ActorID: "system:rollup",
				Notes: fmt.Sprintf("regenerate rule=%s value=%.4f dari 4 TW", rule, val)})
			created++
			continue
		}
		if !errors.Is(exErr, gorm.ErrRecordNotFound) {
			continue
		}
		a := entity.AchievementRecord{
			IndicatorID: k.ind, UnitID: k.unit, UnitType: k.utype, PeriodID: ann.ID,
			Source: "rollup", Status: "draft", CalculatedValue: val,
		}
		// snapshot formula dari record TW terakhir (immutability historis)
		if last := lastBySeq(recs, seqOf); last != nil && last.FormulaID != nil {
			f := *last.FormulaID
			a.FormulaID = &f
		}
		s.applyRollupTarget(&a, &ind)
		if err := s.db.Create(&a).Error; err != nil {
			continue
		}
		s.db.Create(&entity.WorkflowLog{AchievementID: a.ID, Action: "rollup", ActorID: "system:rollup",
			Notes: fmt.Sprintf("generate rule=%s value=%.4f dari 4 TW (%s)", rule, val, ann.Label)})
		created++
	}
	if created > 0 && s.notifier != nil {
		s.notifier.NotifyAdmins("period", fmt.Sprintf("Rollup tahunan %d siap direview", year),
			fmt.Sprintf("%d capaian tahunan digenerate (draft) dari agregasi 4 triwulan. Review & publish di menu Capaian.", created),
			"period", ann.ID)
	}
	return created, nil
}

func lastBySeq(recs []entity.AchievementRecord, seqOf map[string]int) *entity.AchievementRecord {
	var best *entity.AchievementRecord
	bestSeq := -1
	for i := range recs {
		if seqOf[recs[i].PeriodID] > bestSeq {
			bestSeq = seqOf[recs[i].PeriodID]
			best = &recs[i]
		}
	}
	return best
}

func aggregateRollup(recs []entity.AchievementRecord, seqOf map[string]int, rule string) float64 {
	switch rule {
	case "sum":
		t := 0.0
		for _, r := range recs {
			t += r.CalculatedValue
		}
		return t
	case "last":
		if l := lastBySeq(recs, seqOf); l != nil {
			return l.CalculatedValue
		}
		return 0
	default: // avg
		t := 0.0
		for _, r := range recs {
			t += r.CalculatedValue
		}
		return t / float64(len(recs))
	}
}

// applyRollupTarget — pct + warna threshold vs target tahunan (bila ada).
func (s *PeriodService) applyRollupTarget(a *entity.AchievementRecord, ind *entity.IndicatorDefinition) {
	var tgt entity.PerformanceTarget
	if err := s.db.Where("indicator_id = ? AND unit_id = ? AND period_id = ?",
		a.IndicatorID, a.UnitID, a.PeriodID).First(&tgt).Error; err != nil || tgt.TargetValue == 0 {
		a.AchievementPct = nil
		a.TargetValue = nil
		a.ThresholdColor = ""
		return
	}
	pct := a.CalculatedValue / tgt.TargetValue * 100
	a.AchievementPct = &pct
	a.TargetValue = &tgt.TargetValue
	color := "green"
	var th entity.IndicatorThreshold
	if s.db.Where("indicator_id = ?", a.IndicatorID).First(&th).Error == nil {
		switch {
		case pct < th.RedBelow:
			color = "red"
		case pct < th.YellowBelow:
			color = "yellow"
		}
	} else {
		switch {
		case pct < 50:
			color = "red"
		case pct < 75:
			color = "yellow"
		}
	}
	a.ThresholdColor = color
}

// RunLifecycleTick — satu putaran scheduler (dipanggil boot + tiap interval):
//  1. provision tahun berjalan + berikutnya (idempoten)
//  2. open dengan due lewat -> grace (+notifikasi admin)
//  3. grace hari-H -> reminder harian; grace lewat -> auto-close
func (s *PeriodService) RunLifecycleTick() (graceN, closeN int, err error) {
	now := time.Now()
	if err := EnsureYearPeriods(s.db, now.Year()); err != nil {
		return 0, 0, err
	}
	if err := EnsureYearPeriods(s.db, now.Year()+1); err != nil {
		return 0, 0, err
	}

	// open -> grace
	var duePassed []entity.Period
	if err := s.db.Where("status = 'open' AND due_date IS NOT NULL AND due_date < ?", now).Find(&duePassed).Error; err != nil {
		return 0, 0, err
	}
	for _, p := range duePassed {
		if err := s.db.Model(&entity.Period{}).Where("id = ?", p.ID).Update("status", "grace").Error; err != nil {
			continue
		}
		graceN++
		if s.notifier != nil {
			s.notifier.NotifyAdmins("period", fmt.Sprintf("Periode %s melewati batas input", p.Label),
				fmt.Sprintf("Status kini GRACE — input terlambat masih diterima hingga %s, lalu auto-close.",
					p.GraceEnd().Format("02 Jan 2006")), "period", p.ID)
		}
	}

	// grace -> auto-close / reminder
	var inGrace []entity.Period
	if err := s.db.Where("status = 'grace'").Find(&inGrace).Error; err != nil {
		return graceN, 0, err
	}
	today := now.Format("2006-01-02")
	for _, p := range inGrace {
		if now.After(p.GraceEnd()) {
			if _, err := s.ClosePeriod(p.ID, "", "", true); err == nil {
				closeN++
			} else {
				log.Printf("[period] auto-close %s gagal: %v", p.Label, err)
			}
			continue
		}
		// reminder harian (dedupe in-memory; restart boleh kirim ulang 1x)
		if s.graceNoticed[p.ID] != today {
			s.graceNoticed[p.ID] = today
			var pending int64
			s.db.Model(&entity.AchievementRecord{}).
				Where("period_id = ? AND status = ? AND deleted_at IS NULL", p.ID, "draft").Count(&pending)
			if s.notifier != nil {
				s.notifier.NotifyAdmins("period", fmt.Sprintf("Periode %s menunggu tutup buku", p.Label),
					fmt.Sprintf("Auto-close %s. %d capaian masih draft — ingatkan unit penginput.",
						p.GraceEnd().Format("02 Jan 2006"), pending), "period", p.ID)
			}
		}
	}
	return graceN, closeN, nil
}

// StartScheduler — background loop; return func stop.
func (s *PeriodService) StartScheduler(interval time.Duration) func() {
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if _, _, err := s.RunLifecycleTick(); err != nil {
					log.Printf("[period] lifecycle tick error: %v", err)
				}
			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}
