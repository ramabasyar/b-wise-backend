// Package teamsync menjaga tabel teams tetap menjadi cermin pohon unit Human Capital
// (keputusan D-15 blueprint Pusat Layanan v1.1):
//   - setiap unit HC aktif dan non-group otomatis menjadi tim;
//   - unit dinonaktifkan di HC -> tim dinonaktifkan (tidak pernah dihapus, R-11);
//   - unit baru muncul tanpa perubahan kode.
package teamsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/adapter/config"
	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"gorm.io/gorm"
)

type Syncer struct {
	db     *gorm.DB
	hc     config.HCConfig
	sso    config.SSOConfig
	client *http.Client
	logFn  func(format string, args ...interface{})
}

// New — konstruktor sinkronisator tim.
func New(db *gorm.DB, hc config.HCConfig, sso config.SSOConfig, logFn func(format string, args ...interface{})) *Syncer {
	if logFn == nil {
		logFn = func(string, ...interface{}) {}
	}
	return &Syncer{db: db, hc: hc, sso: sso, client: &http.Client{Timeout: 20 * time.Second}, logFn: logFn}
}

// Enabled — sinkron hanya jalan bila URL HC + kredensial service terisi.
func (s *Syncer) Enabled() bool {
	return s.hc.URL != "" && s.sso.URL != "" && s.sso.ServiceClientID != ""
}

// hcDepartment — representasi unit HC dari GET /api/departments.
type hcDepartment struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	OrgType  string `json:"org_type"`
	IsGroup  bool   `json:"is_group"`
	IsActive bool   `json:"is_active"`
}

// getServiceToken — POST SSO /api/oauth/token (client credentials), pola permissionsync.
func (s *Syncer) getServiceToken() (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id":     s.sso.ServiceClientID,
		"client_secret": s.sso.ServiceClientSecret,
		"grant_type":    "client_credentials",
	})
	resp, err := s.client.Post(s.sso.URL+"/api/oauth/token", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.Data.AccessToken != "" {
		return r.Data.AccessToken, nil
	}
	if r.AccessToken != "" {
		return r.AccessToken, nil
	}
	return "", fmt.Errorf("service token: respons tanpa access_token: %s", truncate(string(body), 200))
}

// fetchDepartments — seluruh pohon unit dari HC (envelope B-Wise, dua bentuk).
func (s *Syncer) fetchDepartments(token string) ([]hcDepartment, error) {
	req, err := http.NewRequest(http.MethodGet, s.hc.URL+"/api/departments", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET departments: status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	rows := parseRows(env.Data)
	if rows == nil {
		return nil, fmt.Errorf("GET departments: bentuk envelope tidak dikenali: %s", truncate(string(body), 200))
	}
	return rows, nil
}

func parseRows(raw json.RawMessage) []hcDepartment {
	var outer struct {
		Data []hcDepartment `json:"data"`
	}
	if json.Unmarshal(raw, &outer) == nil && outer.Data != nil {
		return outer.Data
	}
	var direct []hcDepartment
	if json.Unmarshal(raw, &direct) == nil && direct != nil {
		return direct
	}
	return nil
}

// SyncOnce — satu putaran sinkronisasi tim dari pohon unit HC.
func (s *Syncer) SyncOnce() error {
	token, err := s.getServiceToken()
	if err != nil {
		return err
	}
	depts, err := s.fetchDepartments(token)
	if err != nil {
		return err
	}

	now := time.Now()
	created, updated := 0, 0
	seen := make(map[string]bool, len(depts))

	for _, d := range depts {
		if d.IsGroup || d.ID == "" {
			continue // node grup (universitas/fakultas/rektorat) bukan tim layanan
		}
		seen[d.ID] = true
		fo := d.Code == s.hc.FrontOfficeCode

		var team entity.Team
		err := s.db.Where("hc_department_id = ?", d.ID).First(&team).Error
		switch {
		case err == gorm.ErrRecordNotFound:
			team = entity.Team{
				Base:           entity.Base{},
				HCDepartmentID: d.ID,
				Code:           d.Code,
				Name:           d.Name,
				OrgType:        d.OrgType,
				IsFrontOffice:  fo,
				IsActive:       d.IsActive,
				SyncedAt:       now,
			}
			if err := s.db.Create(&team).Error; err != nil {
				return fmt.Errorf("create tim %s: %w", d.Code, err)
			}
			created++
		case err != nil:
			return err
		default:
			changes := map[string]interface{}{
				"code":            d.Code,
				"name":            d.Name,
				"org_type":        d.OrgType,
				"is_front_office": fo,
				"is_active":       d.IsActive,
				"synced_at":       now,
			}
			if err := s.db.Model(&team).Updates(changes).Error; err != nil {
				return fmt.Errorf("update tim %s: %w", d.Code, err)
			}
			updated++
		}
	}

	// Unit hilang/dihapus dari HC -> tim dinonaktifkan defensif, tidak dihapus (R-11).
	if len(seen) > 0 {
		keys := make([]string, 0, len(seen))
		for k := range seen {
			keys = append(keys, k)
		}
		var missing []entity.Team
		if err := s.db.Where("hc_department_id NOT IN ?", keys).Find(&missing).Error; err == nil {
			for i := range missing {
				if missing[i].IsActive {
					if err := s.db.Model(&missing[i]).Updates(map[string]interface{}{"is_active": false, "synced_at": now}).Error; err == nil {
						updated++
					}
				}
			}
		}
	}

	s.logFn("[team-sync] selesai: unit HC=%d tim_baru=%d diperbarui=%d (unit aktif non-group -> tim aktif)", len(depts), created, updated)
	return nil
}

// RunInBackground — sinkron saat boot (retry dengan backoff) lalu berkala.
func (s *Syncer) RunInBackground(stopCh <-chan struct{}, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	for attempt := 1; attempt <= 10; attempt++ {
		if err := s.SyncOnce(); err != nil {
			s.logFn("[team-sync] gagal (percobaan %d): %v", attempt, err)
		} else {
			break
		}
		backoff := time.Duration(attempt) * 5 * time.Second
		if backoff > time.Minute {
			backoff = time.Minute
		}
		select {
		case <-stopCh:
			return
		case <-time.After(backoff):
		}
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
			if err := s.SyncOnce(); err != nil {
				s.logFn("[team-sync] gagal (berkala): %v", err)
			}
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
