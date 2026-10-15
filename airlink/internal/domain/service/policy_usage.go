package service

import (
	"fmt"
)

// DailyUsagePoint - pemakaian satu hari (bytes).
type DailyUsagePoint struct {
	Date  string `json:"date"`
	Bytes int64  `json:"bytes"`
}

// TopUser - user dengan pemakaian terbesar.
type TopUser struct {
	Username string `json:"username"`
	Bytes    int64  `json:"bytes"`
}

// NearingUser - user yang mendekati / melewati kuota harian.
type NearingUser struct {
	Username     string `json:"username"`
	QuotaDailyMB int64  `json:"quota_daily_mb"`
	TodayBytes   int64  `json:"today_bytes"`
	Percent      int    `json:"percent"`
}

// UsageSummary - ringkasan pemakaian jaringan utk dashboard.
type PolicyUsageSummary struct {
	TotalUsers     int64         `json:"total_users"`
	ActiveSessions int64         `json:"active_sessions"`
	Suspended      int64         `json:"suspended_users"`
	TodayBytes     int64         `json:"today_bytes"`
	MonthBytes     int64         `json:"month_bytes"`
	TopUsers       []TopUser     `json:"top_users"`
	NearingQuota   []NearingUser `json:"nearing_quota"`
}

// DailyUsage - tren pemakaian harian user N hari terakhir (seri menerus, termasuk nol).
func (s *PolicyService) DailyUsage(username string, days int) ([]DailyUsagePoint, error) {
	if !validUsername(username) {
		return nil, fmt.Errorf("username tidak valid")
	}
	if days <= 0 || days > 90 {
		days = 30
	}
	rows, err := s.db.Query(`
		SELECT to_char(d.day, 'YYYY-MM-DD'), COALESCE(SUM(r.acctinputoctets + r.acctoutputoctets), 0)
		FROM generate_series(
			date_trunc('day', now()) - (($2::int - 1) * interval '1 day'),
			date_trunc('day', now()),
			interval '1 day') AS d(day)
		LEFT JOIN radacct r
			ON r.username = $1
			AND r.acctstarttime >= d.day
			AND r.acctstarttime < d.day + interval '1 day'
		GROUP BY d.day
		ORDER BY d.day`, username, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DailyUsagePoint{}
	for rows.Next() {
		var p DailyUsagePoint
		if err := rows.Scan(&p.Date, &p.Bytes); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UsageSummary - agregat ringkas untuk kartu dashboard.
func (s *PolicyService) UsageSummary() (*PolicyUsageSummary, error) {
	u := &PolicyUsageSummary{TopUsers: []TopUser{}, NearingQuota: []NearingUser{}}
	queries := []struct {
		dest *int64
		sql  string
	}{
		{&u.TotalUsers, `SELECT COUNT(DISTINCT username) FROM radacct`},
		{&u.ActiveSessions, `SELECT COUNT(*) FROM radacct WHERE acctstoptime IS NULL`},
		{&u.Suspended, `SELECT COUNT(*) FROM airlink_user_policy WHERE status='suspended'`},
		{&u.TodayBytes, `SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE acctstarttime >= date_trunc('day', now())`},
		{&u.MonthBytes, `SELECT COALESCE(SUM(acctinputoctets+acctoutputoctets),0) FROM radacct WHERE acctstarttime >= date_trunc('month', now())`},
	}
	for _, q := range queries {
		if err := s.db.QueryRow(q.sql).Scan(q.dest); err != nil {
			return nil, err
		}
	}
	rows, err := s.db.Query(`
		SELECT username, COALESCE(SUM(acctinputoctets+acctoutputoctets),0)
		FROM radacct WHERE acctstarttime >= date_trunc('month', now())
		GROUP BY username ORDER BY 2 DESC LIMIT 5`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t TopUser
			if rows.Scan(&t.Username, &t.Bytes) == nil {
				u.TopUsers = append(u.TopUsers, t)
			}
		}
		rows.Close()
	}
	rows2, err := s.db.Query(`
		SELECT p.username, p.quota_daily_mb,
			(SELECT COALESCE(SUM(r.acctinputoctets+r.acctoutputoctets),0) FROM radacct r
			 WHERE r.username=p.username AND r.acctstarttime >= date_trunc('day', now()))
		FROM airlink_user_policy p
		WHERE p.quota_daily_mb IS NOT NULL AND p.quota_daily_mb > 0 AND p.status='active'
		  AND (SELECT COALESCE(SUM(r.acctinputoctets+r.acctoutputoctets),0) FROM radacct r
		       WHERE r.username=p.username AND r.acctstarttime >= date_trunc('day', now()))
		      >= p.quota_daily_mb * 1048576 * 0.8
		ORDER BY 3 DESC LIMIT 10`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var n NearingUser
			if rows2.Scan(&n.Username, &n.QuotaDailyMB, &n.TodayBytes) == nil {
				limit := n.QuotaDailyMB * 1048576
				if limit > 0 {
					n.Percent = int(float64(n.TodayBytes) * 100 / float64(limit))
				}
				u.NearingQuota = append(u.NearingQuota, n)
			}
		}
	}
	return u, nil
}
