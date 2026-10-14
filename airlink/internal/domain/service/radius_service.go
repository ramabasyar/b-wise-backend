package service

import (
	"database/sql"
	"fmt"
	"time"

	"airlink/internal/domain"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// RadiusService — read-only access ke DB radius FreeRADIUS
type RadiusService struct {
	db *sql.DB // raw connection ke DB radius (bukan GORM — read-only)
}

func NewRadiusService(dsn string) (*RadiusService, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("radius db connect: %w", err)
	}
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("radius db ping: %w", err)
	}
	return &RadiusService{db: db}, nil
}

func (s *RadiusService) Close() error { return s.db.Close() }

// GetActiveSessions — sesi WiFi yang masih aktif (acctstoptime IS NULL)
func (s *RadiusService) GetActiveSessions(username string, limit, offset int) ([]domain.RadAcct, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where := "WHERE acctstoptime IS NULL"
	args := []interface{}{}
	if username != "" {
		where += " AND username = $1"
		args = append(args, username)
	}

	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM radacct "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf(`SELECT COALESCE(acctsessionid,''), COALESCE(acctuniqueid,''), COALESCE(username,''),
		COALESCE(nasipaddress::text,''), calledstationid, callingstationid, framedipaddress::text,
		acctstarttime, acctupdatetime, acctstoptime
		FROM radacct %s ORDER BY acctstarttime DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sessions []domain.RadAcct
	for rows.Next() {
		var a domain.RadAcct
		err := rows.Scan(&a.AcctSessionID, &a.AcctUniqueID, &a.UserName,
			&a.NASIPAddress, &a.CalledStationID, &a.CallingStationID, &a.FramedIPAddress,
			&a.AcctStartTime, &a.AcctUpdateTime, &a.AcctStopTime)
		if err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, a)
	}
	return sessions, total, rows.Err()
}

// GetSessionHistory — riwayat sesi (termasuk yang sudah selesai)
func (s *RadiusService) GetSessionHistory(username string, startTime, endTime time.Time, limit, offset int) ([]domain.RadAcct, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}
	if startTime.IsZero() {
		startTime = endTime.AddDate(0, 0, -7)
	}

	where := "WHERE acctstarttime >= $1 AND acctstarttime <= $2"
	args := []interface{}{startTime, endTime}
	if username != "" {
		where += fmt.Sprintf(" AND username = $%d", len(args)+1)
		args = append(args, username)
	}

	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM radacct "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf(`SELECT COALESCE(acctsessionid,''), COALESCE(acctuniqueid,''), COALESCE(username,''),
		COALESCE(nasipaddress::text,''), calledstationid, callingstationid, framedipaddress::text,
		acctstarttime, acctupdatetime, acctstoptime
		FROM radacct %s ORDER BY acctstarttime DESC LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sessions []domain.RadAcct
	for rows.Next() {
		var a domain.RadAcct
		err := rows.Scan(&a.AcctSessionID, &a.AcctUniqueID, &a.UserName,
			&a.NASIPAddress, &a.CalledStationID, &a.CallingStationID, &a.FramedIPAddress,
			&a.AcctStartTime, &a.AcctUpdateTime, &a.AcctStopTime)
		if err != nil {
			return nil, 0, err
		}
		sessions = append(sessions, a)
	}
	return sessions, total, rows.Err()
}

// GetAuthLogs — hasil autentikasi terakhir dari radpostauth
func (s *RadiusService) GetAuthLogs(username string, limit, offset int) ([]domain.RadPostAuth, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	where := "WHERE 1=1"
	args := []interface{}{}
	if username != "" {
		where += " AND username = $1"
		args = append(args, username)
	}

	var total int64
	err := s.db.QueryRow("SELECT COUNT(*) FROM radpostauth "+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf("SELECT id, username, reply, authdate FROM radpostauth %s ORDER BY authdate DESC LIMIT $%d OFFSET $%d", where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []domain.RadPostAuth
	for rows.Next() {
		var l domain.RadPostAuth
		err := rows.Scan(&l.ID, &l.UserName, &l.Reply, &l.AuthDate)
		if err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// GetHealth — status kesehatan
func (s *RadiusService) GetHealth() domain.HealthStatus {
	h := domain.HealthStatus{}
	h.RadiusDBUp = s.db.Ping() == nil
	if !h.RadiusDBUp {
		return h
	}

	s.db.QueryRow("SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL").Scan(&h.ActiveSessions)

	var lastReply *string
	row := s.db.QueryRow("SELECT reply FROM radpostauth ORDER BY authdate DESC LIMIT 1")
	if err := row.Scan(&lastReply); err == nil && lastReply != nil {
		h.LastAuthReply = lastReply
	}
	return h
}

// ActiveSessionInfo — info sesi aktif untuk CoA disconnect
type ActiveSessionInfo struct {
	AcctSessionID    string
	NASIPAddress     string
	CallingStationID string
	UserName         string
}

// GetActiveSessionsForUser — sesi aktif milik satu user (untuk CoA revoke)
func (s *RadiusService) GetActiveSessionsForUser(username string) ([]ActiveSessionInfo, error) {
	rows, err := s.db.Query(
		"SELECT acctsessionid, nasipaddress::text, COALESCE(callingstationid,''), username FROM radacct WHERE acctstoptime IS NULL AND username = $1",
		username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []ActiveSessionInfo
	for rows.Next() {
		var info ActiveSessionInfo
		if err := rows.Scan(&info.AcctSessionID, &info.NASIPAddress, &info.CallingStationID, &info.UserName); err != nil {
			return nil, err
		}
		sessions = append(sessions, info)
	}
	return sessions, rows.Err()
}

// UsageSummary — ringkasan penggunaan bandwidth per user
type UsageSummary struct {
	UserName      string  `json:"username"`
	TotalUpload   int64   `json:"total_upload_bytes"`
	TotalDownload int64   `json:"total_download_bytes"`
	TotalSessions int     `json:"total_sessions"`
	TotalDuration int64   `json:"total_duration_sec"`
	AvgSpeedMbps  float64 `json:"avg_speed_mbps"`
	PeakSpeedMbps float64 `json:"peak_speed_mbps"`
}

// UsageSession — detail bandwidth per sesi
type UsageSession struct {
	AcctSessionID string     `json:"acct_session_id"`
	UserName      string     `json:"username"`
	UploadBytes   int64      `json:"upload_bytes"`
	DownloadBytes int64      `json:"download_bytes"`
	DurationSec   int64      `json:"duration_sec"`
	AvgSpeedMbps  float64    `json:"avg_speed_mbps"`
	StartTime     time.Time  `json:"start_time"`
	StopTime      *time.Time `json:"stop_time,omitempty"`
	NASIPAddress  string     `json:"nas_ip"`
}

// GetUsageSummary — agregat bandwidth per user (top N)
func (s *RadiusService) GetUsageSummary(startTime, endTime time.Time, limit int) ([]UsageSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `
		SELECT username,
			COALESCE(SUM(acctinputoctets), 0) as total_up,
			COALESCE(SUM(acctoutputoctets), 0) as total_down,
			COUNT(*) as sessions,
			COALESCE(SUM(acctsessiontime), 0) as total_dur
		FROM radacct
		WHERE acctstarttime >= $1 AND acctstarttime <= $2
		GROUP BY username
		ORDER BY (COALESCE(SUM(acctinputoctets), 0) + COALESCE(SUM(acctoutputoctets), 0)) DESC
		LIMIT $3`
	rows, err := s.db.Query(query, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []UsageSummary
	for rows.Next() {
		var u UsageSummary
		if err := rows.Scan(&u.UserName, &u.TotalUpload, &u.TotalDownload, &u.TotalSessions, &u.TotalDuration); err != nil {
			return nil, err
		}
		if u.TotalDuration > 0 {
			totalBytes := float64(u.TotalUpload + u.TotalDownload)
			u.AvgSpeedMbps = (totalBytes * 8) / (float64(u.TotalDuration) * 1000000)
		}
		results = append(results, u)
	}
	return results, rows.Err()
}

// GetPeakSpeed — kecepatan maksimum per user (sesi tercepat)
func (s *RadiusService) GetPeakSpeed(username string, startTime, endTime time.Time) (float64, error) {
	query := `
		SELECT COALESCE(MAX(
			CASE WHEN acctsessiontime > 60 THEN
				(COALESCE(acctinputoctets,0) + COALESCE(acctoutputoctets,0)) * 8.0 / acctsessiontime / 1000000
			ELSE 0 END
		), 0)
		FROM radacct
		WHERE username = $1 AND acctstarttime >= $2 AND acctstarttime <= $3`
	var peak float64
	err := s.db.QueryRow(query, username, startTime, endTime).Scan(&peak)
	return peak, err
}

// GetUserUsageSessions — detail bandwidth per sesi untuk satu user
func (s *RadiusService) GetUserUsageSessions(username string, startTime, endTime time.Time, limit int) ([]UsageSession, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `
		SELECT acctsessionid, username,
			COALESCE(acctinputoctets, 0),
			COALESCE(acctoutputoctets, 0),
			COALESCE(acctsessiontime, 0),
			acctstarttime, acctstoptime, nasipaddress::text
		FROM radacct
		WHERE username = $1 AND acctstarttime >= $2 AND acctstarttime <= $3
		ORDER BY acctstarttime DESC
		LIMIT $4`
	rows, err := s.db.Query(query, username, startTime, endTime, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []UsageSession
	for rows.Next() {
		var u UsageSession
		if err := rows.Scan(&u.AcctSessionID, &u.UserName, &u.UploadBytes, &u.DownloadBytes,
			&u.DurationSec, &u.StartTime, &u.StopTime, &u.NASIPAddress); err != nil {
			return nil, err
		}
		if u.DurationSec > 0 {
			totalBytes := float64(u.UploadBytes + u.DownloadBytes)
			u.AvgSpeedMbps = (totalBytes * 8) / (float64(u.DurationSec) * 1000000)
		}
		results = append(results, u)
	}
	return results, rows.Err()
}
