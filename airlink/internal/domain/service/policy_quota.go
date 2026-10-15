package service

import (
	"context"
	"fmt"
	"regexp"
	"time"
)

var timePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
var dayListPattern = regexp.MustCompile(`^[1-7](,[1-7]){0,6}$`)

// SetQuota - kuota pemakaian per user (MB); NULL = tanpa batas.
func (s *PolicyService) SetQuota(username string, dailyMB, monthlyMB *int64, actor string) error {
	if !validUsername(username) {
		return fmt.Errorf("username tidak valid")
	}
	if dailyMB != nil && *dailyMB <= 0 {
		return fmt.Errorf("kuota harian harus lebih dari 0 MB")
	}
	if monthlyMB != nil && *monthlyMB <= 0 {
		return fmt.Errorf("kuota bulanan harus lebih dari 0 MB")
	}
	var d, m interface{}
	if dailyMB != nil {
		d = *dailyMB
	}
	if monthlyMB != nil {
		m = *monthlyMB
	}
	_, err := s.db.Exec(`
		INSERT INTO airlink_user_policy (username, status, quota_daily_mb, quota_monthly_mb, updated_by, updated_at)
		VALUES ($1, 'active', $2, $3, $4, NOW())
		ON CONFLICT (username) DO UPDATE SET
			quota_daily_mb=$2, quota_monthly_mb=$3, updated_by=$4, updated_at=NOW()`,
		username, d, m, actor)
	if err != nil {
		return fmt.Errorf("gagal menyimpan kuota: %w", err)
	}
	s.logEvent(username, actor, "quota", fmt.Sprintf("daily=%vMB monthly=%vMB", dailyMB, monthlyMB))
	return nil
}

// SetSchedule - jadwal akses; start/end NULL = selalu boleh; days = daftar ISODOW "1,2,3,4,5".
func (s *PolicyService) SetSchedule(username string, start, end *string, days string, actor string) error {
	if !validUsername(username) {
		return fmt.Errorf("username tidak valid")
	}
	if days == "" {
		days = "1,2,3,4,5,6,7"
	}
	if !dayListPattern.MatchString(days) {
		return fmt.Errorf("format hari tidak valid (contoh: 1,2,3,4,5 utk Sen-Jum)")
	}
	if start != nil && !timePattern.MatchString(*start) {
		return fmt.Errorf("format jam harus HH:MM (contoh 07:00)")
	}
	if end != nil && !timePattern.MatchString(*end) {
		return fmt.Errorf("format jam harus HH:MM (contoh 17:00)")
	}
	var st, en interface{}
	if start != nil {
		st = *start
	}
	if end != nil {
		en = *end
	}
	_, err := s.db.Exec(`
		INSERT INTO airlink_user_policy (username, status, access_start, access_end, access_days, updated_by, updated_at)
		VALUES ($1, 'active', $2, $3, $4, $5, NOW())
		ON CONFLICT (username) DO UPDATE SET
			access_start=$2, access_end=$3, access_days=$4, updated_by=$5, updated_at=NOW()`,
		username, st, en, days, actor)
	if err != nil {
		return fmt.Errorf("gagal menyimpan jadwal: %w", err)
	}
	s.logEvent(username, actor, "schedule", fmt.Sprintf("%s-%s hari=%s", deref(start), deref(end), days))
	return nil
}

func deref(p *string) string {
	if p == nil {
		return "-"
	}
	return *p
}

// UserUsage - pemakaian user (bytes) hari ini & bulan ini, dari radacct.
func (s *PolicyService) UserUsage(username string) (today, month int64, err error) {
	err = s.db.QueryRow(`
		SELECT
			(SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE username=$1 AND acctstarttime >= date_trunc('day', now())),
			(SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE username=$1 AND acctstarttime >= date_trunc('month', now()))`,
		username).Scan(&today, &month)
	return
}

// CheckQuotas - auto-suspend user yang lewat kuota; auto-aktifkan lagi saat periode reset.
// Return daftar username yang baru di-suspend / di-aktifkan kembali.
func (s *PolicyService) CheckQuotas() (suspended, lifted []string, err error) {
	// 1) Auto-lift: suspended oleh kuota tapi pemakaian periode berjalan sudah di bawah batas.
	rows, err := s.db.Query(`
		UPDATE airlink_user_policy p SET status='active', auto_suspended_by=NULL,
			suspend_reason=NULL, suspended_by=NULL, suspended_at=NULL, updated_by='system', updated_at=NOW()
		WHERE p.status='suspended' AND p.auto_suspended_by LIKE 'quota%'
		  AND NOT (
		    (p.auto_suspended_by='quota_daily' AND p.quota_daily_mb IS NOT NULL AND
		      (SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct r
		       WHERE r.username=p.username AND r.acctstarttime >= date_trunc('day', now())) >= p.quota_daily_mb*1048576)
		    OR
		    (p.auto_suspended_by='quota_monthly' AND p.quota_monthly_mb IS NOT NULL AND
		      (SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct r
		       WHERE r.username=p.username AND r.acctstarttime >= date_trunc('month', now())) >= p.quota_monthly_mb*1048576)
		  )
		RETURNING p.username`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			lifted = append(lifted, u)
			s.logEvent(u, "system", "unsuspend", "kuota periode berjalan sudah reset")
		}
	}
	rows.Close()

	// 2) Auto-suspend harian.
	rows, err = s.db.Query(`
		UPDATE airlink_user_policy p SET status='suspended', auto_suspended_by='quota_daily',
			suspend_reason='Kuota harian terlampaui (otomatis)', suspended_by='system',
			suspended_at=NOW(), updated_by='system', updated_at=NOW()
		WHERE p.status='active' AND p.quota_daily_mb IS NOT NULL AND
		  (SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct r
		   WHERE r.username=p.username AND r.acctstarttime >= date_trunc('day', now())) >= p.quota_daily_mb*1048576
		RETURNING p.username`)
	if err != nil {
		return suspended, lifted, err
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			suspended = append(suspended, u)
			s.logEvent(u, "system", "suspend", "kuota harian terlampaui")
			_ = s.kickActive(u)
		}
	}
	rows.Close()

	// 3) Auto-suspend bulanan (belum tersuspend).
	rows, err = s.db.Query(`
		UPDATE airlink_user_policy p SET status='suspended', auto_suspended_by='quota_monthly',
			suspend_reason='Kuota bulanan terlampaui (otomatis)', suspended_by='system',
			suspended_at=NOW(), updated_by='system', updated_at=NOW()
		WHERE p.status='active' AND p.quota_monthly_mb IS NOT NULL AND
		  (SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct r
		   WHERE r.username=p.username AND r.acctstarttime >= date_trunc('month', now())) >= p.quota_monthly_mb*1048576
		RETURNING p.username`)
	if err != nil {
		return suspended, lifted, err
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err == nil {
			suspended = append(suspended, u)
			s.logEvent(u, "system", "suspend", "kuota bulanan terlampaui")
			_ = s.kickActive(u)
		}
	}
	rows.Close()
	return suspended, lifted, nil
}

// RunQuotaReconciler - loop periodik pemeriksaan kuota (jalan tanpa MikroTik).
func (s *PolicyService) RunQuotaReconciler(ctx context.Context, interval time.Duration, logf func(string, ...interface{})) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	logf("quota reconciler mulai (interval %s)", interval)
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					logf("panic quota reconciler: %v", r)
				}
			}()
			susp, lifted, err := s.CheckQuotas()
			if err != nil {
				logf("check kuota: %v", err)
				return
			}
			for _, u := range susp {
				logf("auto-suspend kuota: %s", u)
			}
			for _, u := range lifted {
				logf("auto-aktif kembali: %s", u)
			}
		}()
		select {
		case <-ctx.Done():
			logf("quota reconciler berhenti")
			return
		case <-t.C:
		}
	}
}
