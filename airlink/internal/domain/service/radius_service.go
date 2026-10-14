package service

import (
	"database/sql"
	"fmt"
	"time"

	"airlink/internal/domain"
)

// RadiusService — read-only access ke DB radius FreeRADIUS
type RadiusService struct {
	db *sql.DB // raw connection ke DB radius (bukan GORM — read-only)
}

func NewRadiusService(dsn string) (*RadiusService, error) {
	db, err := sql.Open("postgres", dsn)
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
		COALESCE(nasipaddress,''), calledstationid, callingstationid, framedipaddress,
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
		COALESCE(nasipaddress,''), calledstationid, callingstationid, framedipaddress,
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
