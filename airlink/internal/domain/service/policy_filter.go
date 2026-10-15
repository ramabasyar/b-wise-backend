package service

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FilterProfile - profil filter konten: block = larang daftar domain,
// allow = hanya izinkan daftar domain (lainnya NXDOMAIN).
type FilterProfile struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Mode      string    `json:"mode"`
	Descr     string    `json:"descr,omitempty"`
	Domains   []string  `json:"domains"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

var domainPattern = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
var filterNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_. -]{0,59}$`)

func normalizeDomain(d string) (string, bool) {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.Trim(d, ".")
	if len(d) < 3 || len(d) > 253 {
		return "", false
	}
	d = strings.TrimPrefix(d, "*.")
	if !domainPattern.MatchString(d) {
		return "", false
	}
	return d, true
}

func (s *PolicyService) ListFilters() ([]FilterProfile, error) {
	rows, err := s.db.Query(`
		SELECT p.id, p.name, p.mode, COALESCE(p.descr,''), p.created_at, p.updated_at,
			COALESCE(array_agg(d.domain ORDER BY d.domain) FILTER (WHERE d.domain IS NOT NULL), '{}')
		FROM airlink_filter_profiles p
		LEFT JOIN airlink_filter_domains d ON d.profile_id = p.id
		GROUP BY p.id ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FilterProfile
	for rows.Next() {
		var f FilterProfile
		var domains []byte
		if err := rows.Scan(&f.ID, &f.Name, &f.Mode, &f.Descr, &f.CreatedAt, &f.UpdatedAt, &domains); err != nil {
			return nil, err
		}
		f.Domains = parsePGTextArray(domains)
		out = append(out, f)
	}
	return out, rows.Err()
}

func parsePGTextArray(raw []byte) []string {
	s := strings.Trim(string(raw), "{}")
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.Trim(parts[i], "\"")
	}
	return parts
}

func (s *PolicyService) sanitizeDomains(domains []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range domains {
		norm, ok := normalizeDomain(d)
		if !ok {
			return nil, fmt.Errorf("domain tidak valid: %q", d)
		}
		if !seen[norm] {
			seen[norm] = true
			out = append(out, norm)
		}
	}
	if len(out) > 500 {
		return nil, fmt.Errorf("maksimal 500 domain per profil")
	}
	return out, nil
}

func (s *PolicyService) CreateFilter(name, mode string, domains []string, descr string) (*FilterProfile, error) {
	if !filterNamePattern.MatchString(name) {
		return nil, fmt.Errorf("nama profil filter tidak valid")
	}
	if mode != "block" && mode != "allow" {
		return nil, fmt.Errorf("mode hanya block atau allow")
	}
	doms, err := s.sanitizeDomains(domains)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`
		INSERT INTO airlink_filter_profiles (name, mode, descr) VALUES ($1,$2,$3) RETURNING id`,
		name, mode, descr).Scan(&id); err != nil {
		return nil, fmt.Errorf("gagal membuat profil filter (nama duplikat?): %w", err)
	}
	for _, d := range doms {
		if _, err := tx.Exec(`INSERT INTO airlink_filter_domains (profile_id, domain) VALUES ($1,$2)`, id, d); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &FilterProfile{ID: id, Name: name, Mode: mode, Descr: descr, Domains: doms,
		CreatedAt: time.Now(), UpdatedAt: time.Now()}, nil
}

func (s *PolicyService) UpdateFilter(id int64, name, mode string, domains []string, descr string) error {
	if !filterNamePattern.MatchString(name) {
		return fmt.Errorf("nama profil filter tidak valid")
	}
	if mode != "block" && mode != "allow" {
		return fmt.Errorf("mode hanya block atau allow")
	}
	doms, err := s.sanitizeDomains(domains)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE airlink_filter_profiles SET name=$2, mode=$3, descr=$4, updated_at=NOW() WHERE id=$1`,
		id, name, mode, descr)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM airlink_filter_domains WHERE profile_id=$1`, id); err != nil {
		return err
	}
	for _, d := range doms {
		if _, err := tx.Exec(`INSERT INTO airlink_filter_domains (profile_id, domain) VALUES ($1,$2)`, id, d); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PolicyService) DeleteFilter(id int64) error {
	var inUse int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM airlink_user_policy WHERE filter_profile_id=$1`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse > 0 {
		return fmt.Errorf("profil filter masih dipakai %d user; lepaskan dulu", inUse)
	}
	res, err := s.db.Exec(`DELETE FROM airlink_filter_profiles WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetUserFilter mengaitkan profil filter ke user; NULL = tanpa filter.
func (s *PolicyService) SetUserFilter(username string, profileID *int64, actor string) (*UserPolicy, error) {
	if !validUsername(username) {
		return nil, fmt.Errorf("username tidak valid")
	}
	if profileID != nil {
		var exists bool
		if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM airlink_filter_profiles WHERE id=$1)`, *profileID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("profil filter tidak ditemukan")
		}
	}
	var pid interface{}
	if profileID != nil {
		pid = *profileID
	}
	_, err := s.db.Exec(`
		INSERT INTO airlink_user_policy (username, status, filter_profile_id, updated_by, updated_at)
		VALUES ($1, 'active', $2, $3, NOW())
		ON CONFLICT (username) DO UPDATE SET
			filter_profile_id=$2, updated_by=$3, updated_at=NOW()`,
		username, pid, actor)
	if err != nil {
		return nil, fmt.Errorf("gagal menyimpan filter user: %w", err)
	}
	s.logEvent(username, actor, "filter", fmt.Sprintf("profile_id=%v", pid))
	return s.GetUser(username)
}
