// Package permissionsync membaca perm.manifest.yaml dan melakukan self-register
// ke Permission Service saat service boot (best-effort, idempotent).
//
// Alur:
//
//	service start → baca perm.manifest.yaml → ambil service token dari SSO
//	→ POST {PERMISSION_SERVICE_URL}/api/services/self/sync → selesai.
//
// Gagal sync TIDAK menghentikan service (hanya log warning + retry berkala).
package permissionsync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/rama/b-wise/iku/internal/adapter/config"
	"gopkg.in/yaml.v3"
)

// Manifest — deklarasi permissions & menu service ini (sumber kebenaran di repo).
// File: perm.manifest.yaml di root service. Ter-commit ke git, ter-review bareng code.
type Manifest struct {
	Service struct {
		Name        string `yaml:"name" json:"name"`
		Description string `yaml:"description" json:"description"`
		BaseURL     string `yaml:"base_url" json:"base_url"`
		SyncEnabled *bool  `yaml:"sync_enabled" json:"sync_enabled"` // default true; false = jangan auto-sync
	} `yaml:"service" json:"service"`
	Permissions []struct {
		Permission  string `yaml:"permission" json:"permission"`
		Description string `yaml:"description" json:"description"`
	} `yaml:"permissions" json:"permissions"`
	Menu []ManifestMenuItem `yaml:"menu" json:"menu"`
}

type ManifestMenuItem struct {
	Label              string             `yaml:"label" json:"label"`
	Path               string             `yaml:"path" json:"path"`
	Icon               string             `yaml:"icon" json:"icon"`
	SortOrder          int                `yaml:"sort_order" json:"sort_order"`
	RequiredPermission string             `yaml:"required_permission" json:"required_permission"`
	Children           []ManifestMenuItem `yaml:"children" json:"children"`
}

// LoadManifest membaca dan memvalidasi perm.manifest.yaml
func LoadManifest(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Service.Name == "" {
		return nil, fmt.Errorf("manifest: service.name wajib diisi")
	}
	return &m, nil
}

// Syncer — client self-sync
type Syncer struct {
	cfg      config.SSOConfig
	manifest *Manifest
	client   *http.Client
	logFn    func(format string, args ...interface{})
}

// New membuat syncer dari config SSO + manifest
func New(cfg config.SSOConfig, m *Manifest, logFn func(format string, args ...interface{})) *Syncer {
	if logFn == nil {
		logFn = func(string, ...interface{}) {}
	}
	return &Syncer{
		cfg:      cfg,
		manifest: m,
		client:   &http.Client{Timeout: 10 * time.Second},
		logFn:    logFn,
	}
}

// Enabled — sync aktif jika manifest tidak menonaktifkan & config lengkap
func (s *Syncer) Enabled() bool {
	if s.manifest.Service.SyncEnabled != nil && !*s.manifest.Service.SyncEnabled {
		return false
	}
	return s.cfg.PermissionServiceURL != "" && s.cfg.URL != "" &&
		s.cfg.ServiceClientID != "" && s.cfg.ServiceName != ""
}

// getServiceToken — POST SSO /api/oauth/token (client credentials)
func (s *Syncer) getServiceToken() (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id":     s.cfg.ServiceClientID,
		"client_secret": s.cfg.ServiceClientSecret,
		"grant_type":    "client_credentials",
	})
	resp, err := s.client.Post(s.cfg.URL+"/api/oauth/token", "application/json", bytes.NewReader(payload))
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
	return r.AccessToken, nil
}

// SyncOnce — satu putaran sinkronisasi. Return error agar bisa di-retry.
func (s *Syncer) SyncOnce() error {
	token, err := s.getServiceToken()
	if err != nil {
		return fmt.Errorf("service token: %w", err)
	}

	// manifest service.name harus == config SERVICE_NAME (double-check sebelum kirim)
	if s.cfg.ServiceName != s.manifest.Service.Name {
		return fmt.Errorf("SERVICE_NAME (%q) != manifest service.name (%q)", s.cfg.ServiceName, s.manifest.Service.Name)
	}

	body, _ := json.Marshal(s.manifest)
	req, _ := http.NewRequest("POST", s.cfg.PermissionServiceURL+"/api/services/self/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("sync rejected (%d): %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}
	s.logFn("[perm-sync] manifest %s tersinkron: %s", s.manifest.Service.Name, string(respBody[:min(len(respBody), 200)]))
	return nil
}

// RunInBackground — retry dengan backoff sampai sukses, lalu berhenti.
// Dipanggil sebagai goroutine dari main.go; berhenti saat stopCh ditutup.
func (s *Syncer) RunInBackground(stopCh <-chan struct{}) {
	go func() {
		backoffs := []time.Duration{2 * time.Second, 5 * time.Second, 15 * time.Second, 30 * time.Second}
		for i := 0; ; i++ {
			err := s.SyncOnce()
			if err == nil {
				return
			}
			s.logFn("[perm-sync] gagal (percobaan %d): %v", i+1, err)
			wait := backoffs[min(i, len(backoffs)-1)]
			select {
			case <-stopCh:
				return
			case <-time.After(wait):
			}
		}
	}()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
