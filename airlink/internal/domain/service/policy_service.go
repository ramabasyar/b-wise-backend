package service

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PolicyService - status akun (suspend), bandwidth, kuota, & jadwal per user WiFi.
// DB: radius (tabel airlink_*). Enforcement auth oleh FreeRADIUS;
// sesi aktif dicoba diputus via CoA, kuota oleh reconciler periodik.
type PolicyService struct {
	db  *sql.DB
	coa *CoAKicker
}

func NewPolicyService(dsn, sshTarget, coaSecret string) (*PolicyService, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("policy db connect: %w", err)
	}
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("policy db ping: %w", err)
	}
	return &PolicyService{db: db, coa: NewCoAKicker(sshTarget, coaSecret)}, nil
}

func (s *PolicyService) Close() error { return s.db.Close() }

// DB mengekspos koneksi bersama (QueueService membaca radacct + policy).
func (s *PolicyService) DB() *sql.DB { return s.db }

type BWProfile struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	RateDown  string    `json:"rate_down"`
	RateUp    string    `json:"rate_up"`
	Descr     string    `json:"descr,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserPolicy struct {
	Username          string     `json:"username"`
	Status            string     `json:"status"`
	SuspendReason     string     `json:"suspend_reason,omitempty"`
	SuspendedBy       string     `json:"suspended_by,omitempty"`
	SuspendedAt       *time.Time `json:"suspended_at,omitempty"`
	SuspendedUntil    *time.Time `json:"suspended_until,omitempty"`
	BWProfileID       *int64     `json:"bw_profile_id,omitempty"`
	BWProfileName     string     `json:"bw_profile_name,omitempty"`
	RateUp            string     `json:"rate_up,omitempty"`
	RateDown          string     `json:"rate_down,omitempty"`
	QuotaDailyMB      *int64     `json:"quota_daily_mb,omitempty"`
	QuotaMonthlyMB    *int64     `json:"quota_monthly_mb,omitempty"`
	AccessStart       *string    `json:"access_start,omitempty"`
	AccessEnd         *string    `json:"access_end,omitempty"`
	AccessDays        string     `json:"access_days,omitempty"`
	AutoSuspendedBy   string     `json:"auto_suspended_by,omitempty"`
	FilterProfileID   *int64     `json:"filter_profile_id,omitempty"`
	FilterProfileName string     `json:"filter_profile_name,omitempty"`
	UpdatedBy         string     `json:"updated_by,omitempty"`
	UpdatedAt         *time.Time `json:"updated_at,omitempty"`
}

type PolicyEvent struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Actor     string    `json:"actor"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// UserRow - baris daftar user: policy + ringkasan sesi + pemakaian.
type UserRow struct {
	UserPolicy
	ActiveSessions int64      `json:"active_sessions"`
	LastSeen       *time.Time `json:"last_seen"`
	TodayBytes     int64      `json:"today_bytes"`
	MonthBytes     int64      `json:"month_bytes"`
}

// SessionRef - sesi aktif yang bisa di-putus via CoA.
type SessionRef struct {
	Username         string `json:"username"`
	AcctSessionID    string `json:"acct_session_id"`
	NASIPAddress     string `json:"nas_ip"`
	CallingStationID string `json:"calling_station_id"`
	FramedIPAddress  string `json:"framed_ip"`
}

var ratePattern = regexp.MustCompile(`^[0-9]{1,6}[KMGT]?$`)
var profileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,59}$`)

func validRate(r string) bool        { return ratePattern.MatchString(r) }
func validProfileName(n string) bool { return profileNamePattern.MatchString(n) }

const policyCols = `COALESCE(p.status,'active'), COALESCE(p.suspend_reason,''), p.suspended_at, p.suspended_until,
	p.bw_profile_id, COALESCE(pr.name,''), COALESCE(p.rate_up,''), COALESCE(p.rate_down,''),
	p.quota_daily_mb, p.quota_monthly_mb, p.access_start::text, p.access_end::text,
	COALESCE(p.access_days,''), COALESCE(p.auto_suspended_by,''),
	p.filter_profile_id, COALESCE(fp.name,''),
	COALESCE(p.suspended_by,''), COALESCE(p.updated_by,''), p.updated_at`

// ListUsers - user dari radacct terbaru + policy + pemakaian (hari ini / bulan ini).
func (s *PolicyService) ListUsers(query string, limit, offset int) ([]UserRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	if query == "" {
		query = "%"
	} else {
		query = "%" + strings.ToLower(query) + "%"
	}
	rows, err := s.db.Query(`
		SELECT u.username, `+policyCols+`, u.active_sessions, u.last_seen,
			COALESCE(td.today_bytes,0), COALESCE(tm.month_bytes,0)
		FROM (
			SELECT username, COUNT(*) FILTER (WHERE acctstoptime IS NULL) AS active_sessions,
				MAX(acctstarttime) AS last_seen
			FROM radacct GROUP BY username ORDER BY MAX(acctstarttime) DESC LIMIT 500
		) u
		LEFT JOIN airlink_user_policy p ON p.username = u.username
		LEFT JOIN airlink_bw_profiles pr ON pr.id = p.bw_profile_id
		LEFT JOIN airlink_filter_profiles fp ON fp.id = p.filter_profile_id
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(r.acctinputoctets+r.acctoutputoctets),0) AS today_bytes
			FROM radacct r WHERE r.username=u.username AND r.acctstarttime >= date_trunc('day', now())
		) td ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(SUM(r.acctinputoctets+r.acctoutputoctets),0) AS month_bytes
			FROM radacct r WHERE r.username=u.username AND r.acctstarttime >= date_trunc('month', now())
		) tm ON true
		WHERE lower(u.username) LIKE $1
		ORDER BY u.last_seen DESC NULLS LAST
		LIMIT $2 OFFSET $3`, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserRow
	for rows.Next() {
		var r UserRow
		if err := rows.Scan(&r.Username,
			&r.Status, &r.SuspendReason, &r.SuspendedAt, &r.SuspendedUntil,
			&r.BWProfileID, &r.BWProfileName, &r.RateUp, &r.RateDown,
			&r.QuotaDailyMB, &r.QuotaMonthlyMB, &r.AccessStart, &r.AccessEnd,
			&r.AccessDays, &r.AutoSuspendedBy, &r.FilterProfileID, &r.FilterProfileName,
			&r.SuspendedBy, &r.UpdatedBy, &r.UpdatedAt,
			&r.ActiveSessions, &r.LastSeen,
			&r.TodayBytes, &r.MonthBytes); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetUser - policy satu user; tanpa baris = default aktif tanpa batasan.
func (s *PolicyService) GetUser(username string) (*UserPolicy, error) {
	var p UserPolicy
	err := s.db.QueryRow(`
		SELECT `+policyCols+` FROM airlink_user_policy p
		LEFT JOIN airlink_bw_profiles pr ON pr.id = p.bw_profile_id
		LEFT JOIN airlink_filter_profiles fp ON fp.id = p.filter_profile_id
		WHERE p.username = $1`, username).Scan(
		&p.Status, &p.SuspendReason, &p.SuspendedAt, &p.SuspendedUntil,
		&p.BWProfileID, &p.BWProfileName, &p.RateUp, &p.RateDown,
		&p.QuotaDailyMB, &p.QuotaMonthlyMB, &p.AccessStart, &p.AccessEnd,
		&p.AccessDays, &p.AutoSuspendedBy, &p.FilterProfileID, &p.FilterProfileName,
		&p.SuspendedBy, &p.UpdatedBy, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return &UserPolicy{Username: username, Status: "active"}, nil
	}
	if err != nil {
		return nil, err
	}
	p.Username = username
	return &p, nil
}

// GetEvents - jejak audit policy user.
func (s *PolicyService) GetEvents(username string, limit int) ([]PolicyEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, username, actor, action, COALESCE(detail,''), created_at
		FROM airlink_policy_events WHERE username = $1
		ORDER BY id DESC LIMIT $2`, username, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyEvent
	for rows.Next() {
		var e PolicyEvent
		if err := rows.Scan(&e.ID, &e.Username, &e.Actor, &e.Action, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *PolicyService) ListProfiles() ([]BWProfile, error) {
	rows, err := s.db.Query(`
		SELECT id, name, rate_down, rate_up, COALESCE(descr,''), created_at, updated_at
		FROM airlink_bw_profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BWProfile
	for rows.Next() {
		var p BWProfile
		if err := rows.Scan(&p.ID, &p.Name, &p.RateDown, &p.RateUp, &p.Descr, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PolicyService) CreateProfile(name, rateDown, rateUp, descr string) (*BWProfile, error) {
	if !validProfileName(name) {
		return nil, fmt.Errorf("nama profil tidak valid")
	}
	if !validRate(rateDown) || !validRate(rateUp) {
		return nil, fmt.Errorf("format rate harus seperti 20M, 10M, atau 512K")
	}
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO airlink_bw_profiles (name, rate_down, rate_up, descr)
		VALUES ($1, $2, $3, $4) RETURNING id`, name, rateDown, rateUp, descr).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat profil (nama duplikat?): %w", err)
	}
	out := &BWProfile{ID: id, Name: name, RateDown: rateDown, RateUp: rateUp, Descr: descr}
	out.CreatedAt, out.UpdatedAt = time.Now(), time.Now()
	return out, nil
}

func (s *PolicyService) UpdateProfile(id int64, name, rateDown, rateUp, descr string) error {
	if !validProfileName(name) {
		return fmt.Errorf("nama profil tidak valid")
	}
	if !validRate(rateDown) || !validRate(rateUp) {
		return fmt.Errorf("format rate harus seperti 20M, 10M, atau 512K")
	}
	res, err := s.db.Exec(`
		UPDATE airlink_bw_profiles SET name=$2, rate_down=$3, rate_up=$4, descr=$5, updated_at=NOW()
		WHERE id=$1`, id, name, rateDown, rateUp, descr)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PolicyService) DeleteProfile(id int64) error {
	var inUse int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM airlink_user_policy WHERE bw_profile_id=$1`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return fmt.Errorf("profil masih dipakai %d user; pindahkan user lain dulu", inUse)
	}
	res, err := s.db.Exec(`DELETE FROM airlink_bw_profiles WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *PolicyService) logEvent(username, actor, action, detail string) {
	if actor == "" {
		actor = "system"
	}
	_, _ = s.db.Exec(`
		INSERT INTO airlink_policy_events (username, actor, action, detail)
		VALUES ($1, $2, $3, $4)`, username, actor, action, detail)
}

// SuspendUser menandai akun suspended (ditolak di auth berikutnya) dan
// mencoba memutus sesi aktif via CoA. Return policy akhir + daftar kegagalan kick.
func (s *PolicyService) SuspendUser(username, reason string, until *time.Time, actor string) (*UserPolicy, []string, error) {
	if !validUsername(username) {
		return nil, nil, fmt.Errorf("username tidak valid")
	}
	if reason == "" {
		reason = "tidak disebutkan"
	}
	detail := reason
	if until != nil {
		detail = fmt.Sprintf("%s (sampai %s)", reason, until.Format(time.RFC3339))
	}
	_, err := s.db.Exec(`
		INSERT INTO airlink_user_policy (username, status, suspend_reason, suspended_by, suspended_at, suspended_until, updated_by, updated_at)
		VALUES ($1, 'suspended', $2, $3, NOW(), $4, $3, NOW())
		ON CONFLICT (username) DO UPDATE SET
			status='suspended', suspend_reason=$2, suspended_by=$3, suspended_at=NOW(),
			suspended_until=$4, updated_by=$3, updated_at=NOW()`,
		username, reason, actor, until)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal menyimpan status suspend: %w", err)
	}
	s.logEvent(username, actor, "suspend", detail)
	kickErrs := s.kickActive(username)
	p, err := s.GetUser(username)
	if err != nil {
		return nil, kickErrs, err
	}
	return p, kickErrs, nil
}

// UnsuspendUser mengaktifkan kembali akun.
func (s *PolicyService) UnsuspendUser(username, actor string) (*UserPolicy, error) {
	if !validUsername(username) {
		return nil, fmt.Errorf("username tidak valid")
	}
	_, err := s.db.Exec(`
		INSERT INTO airlink_user_policy (username, status, updated_by, updated_at)
		VALUES ($1, 'active', $2, NOW())
		ON CONFLICT (username) DO UPDATE SET
			status='active', suspend_reason=NULL, suspended_by=NULL, suspended_at=NULL,
			suspended_until=NULL, auto_suspended_by=NULL, updated_by=$2, updated_at=NOW()`, username, actor)
	if err != nil {
		return nil, fmt.Errorf("gagal mengaktifkan akun: %w", err)
	}
	s.logEvent(username, actor, "unsuspend", "")
	return s.GetUser(username)
}

// KickUser memutus semua sesi aktif user (tanpa mengubah status akun).
func (s *PolicyService) KickUser(username, actor string) ([]string, error) {
	if !validUsername(username) {
		return nil, fmt.Errorf("username tidak valid")
	}
	s.logEvent(username, actor, "kick", "")
	return s.kickActive(username), nil
}

func (s *PolicyService) kickActive(username string) []string {
	rows, err := s.db.Query(`
		SELECT acctsessionid, nasipaddress::text, COALESCE(callingstationid,'')
		FROM radacct WHERE username=$1 AND acctstoptime IS NULL`, username)
	if err != nil {
		return []string{"gagal membaca sesi aktif: " + err.Error()}
	}
	defer rows.Close()
	var errs []string
	for rows.Next() {
		var sid, nas, mac string
		if err := rows.Scan(&sid, &nas, &mac); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := s.coa.Disconnect(username, sid, nas, mac); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", stripCIDR(nas), err))
		}
	}
	return errs
}

// SetBandwidth mengatur override rate per user; string kosong = hapus override.
func (s *PolicyService) SetBandwidth(username string, profileID *int64, rateUp, rateDown string, actor string) (*UserPolicy, error) {
	if !validUsername(username) {
		return nil, fmt.Errorf("username tidak valid")
	}
	if profileID != nil {
		var exists bool
		if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM airlink_bw_profiles WHERE id=$1)`, *profileID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("profil bandwidth tidak ditemukan")
		}
	}
	if rateUp != "" && !validRate(rateUp) {
		return nil, fmt.Errorf("format rate_up tidak valid (contoh: 10M)")
	}
	if rateDown != "" && !validRate(rateDown) {
		return nil, fmt.Errorf("format rate_down tidak valid (contoh: 20M)")
	}
	var up, down interface{}
	if rateUp == "" {
		up = nil
	} else {
		up = rateUp
	}
	if rateDown == "" {
		down = nil
	} else {
		down = rateDown
	}
	var pid interface{}
	if profileID != nil {
		pid = *profileID
	}
	_, err := s.db.Exec(`
		INSERT INTO airlink_user_policy (username, status, bw_profile_id, rate_up, rate_down, updated_by, updated_at)
		VALUES ($1, 'active', $2, $3, $4, $5, NOW())
		ON CONFLICT (username) DO UPDATE SET
			bw_profile_id=$2, rate_up=$3, rate_down=$4, updated_by=$5, updated_at=NOW()`,
		username, pid, up, down, actor)
	if err != nil {
		return nil, fmt.Errorf("gagal menyimpan bandwidth: %w", err)
	}
	detail := fmt.Sprintf("profile_id=%v up=%q down=%q", pid, rateUp, rateDown)
	s.logEvent(username, actor, "bandwidth", detail)
	return s.GetUser(username)
}

// ActiveSessionRefs - semua sesi aktif (untuk reconciler queue MikroTik).
func (s *PolicyService) ActiveSessionRefs() ([]SessionRef, error) {
	rows, err := s.db.Query(`
		SELECT username, acctsessionid, nasipaddress::text, COALESCE(callingstationid,''), COALESCE(framedipaddress::text,'')
		FROM radacct WHERE acctstoptime IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionRef
	for rows.Next() {
		var r SessionRef
		if err := rows.Scan(&r.Username, &r.AcctSessionID, &r.NASIPAddress, &r.CallingStationID, &r.FramedIPAddress); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type RateInfo struct {
	Up        string
	Down      string
	Suspended bool
}

// EffectiveRates - rate efektif user dgn sesi aktif: override > profil > default.
func (s *PolicyService) EffectiveRates(defaultUp, defaultDown string) (map[string]RateInfo, error) {
	rows, err := s.db.Query(`
		SELECT u.username,
			COALESCE(NULLIF(p.rate_up,''),  NULLIF(pr.rate_up,''),  $1),
			COALESCE(NULLIF(p.rate_down,''),NULLIF(pr.rate_down,''),$2),
			(p.status = 'suspended' AND (p.suspended_until IS NULL OR p.suspended_until > NOW()))
		FROM (SELECT DISTINCT username FROM radacct WHERE acctstoptime IS NULL) u
		LEFT JOIN airlink_user_policy p ON p.username = u.username
		LEFT JOIN airlink_bw_profiles pr ON pr.id = p.bw_profile_id
		LEFT JOIN airlink_filter_profiles fp ON fp.id = p.filter_profile_id`, defaultUp, defaultDown)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]RateInfo{}
	for rows.Next() {
		var u string
		var r RateInfo
		if err := rows.Scan(&u, &r.Up, &r.Down, &r.Suspended); err != nil {
			return nil, err
		}
		out[u] = r
	}
	return out, rows.Err()
}
