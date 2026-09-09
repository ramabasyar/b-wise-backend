// Package hcclient — internal client ke human-capital service (unit kerja:
// branch = fakultas/unit, department). IKU memakai service token sendiri
// (client credentials) — HC middleware melewatkan service token.
package hcclient

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Unit struct {
	ID   string `json:"id"`
	Code string `json:"code,omitempty"`
	Name string `json:"name"`
}

type Client struct {
	hcURL             string
	getServiceTokenFn func() (string, error)
	httpClient        *http.Client
	mu                sync.Mutex
	cached            []Unit
	cachedDept        []Unit
	cachedAt          time.Time
	cacheTTL          time.Duration
}

func New(hcURL string, getServiceToken func() (string, error)) *Client {
	return &Client{
		hcURL:             hcURL,
		getServiceTokenFn: getServiceToken,
		httpClient:        &http.Client{Timeout: 10 * time.Second},
		cacheTTL:          5 * time.Minute,
	}
}

func (c *Client) fetch(path string) ([]Unit, error) {
	token, err := c.getServiceTokenFn()
	if err != nil {
		return nil, fmt.Errorf("service token: %w", err)
	}
	req, _ := http.NewRequest("GET", c.hcURL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HC %s: HTTP %d: %s", path, resp.StatusCode, string(body[:min(len(body), 120)]))
	}
	var out struct {
		Data []Unit `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Branches — fakultas/unit (cache 5 menit)
func (c *Client) Branches() ([]Unit, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached != nil && time.Since(c.cachedAt) < c.cacheTTL {
		return c.cached, nil
	}
	list, err := c.fetch("/api/branches")
	if err != nil {
		return nil, err
	}
	c.cached = list
	c.cachedAt = time.Now()
	return list, nil
}

// Departments — unit kerja detail (cache 5 menit)
func (c *Client) Departments() ([]Unit, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cachedDept != nil && time.Since(c.cachedAt) < c.cacheTTL {
		return c.cachedDept, nil
	}
	list, err := c.fetch("/api/departments")
	if err != nil {
		return nil, err
	}
	c.cachedDept = list
	c.cachedAt = time.Now()
	return list, nil
}

// UnitName — resolve nama unit (fallback: id pendek)
func (c *Client) UnitName(id string) string {
	for _, u := range append(c.snapshotBranches(), c.snapshotDepts()...) {
		if u.ID == id {
			return u.Name
		}
	}
	if len(id) > 10 {
		return id[:8] + "…"
	}
	return id
}

func (c *Client) snapshotBranches() []Unit { c.mu.Lock(); defer c.mu.Unlock(); return c.cached }
func (c *Client) snapshotDepts() []Unit    { c.mu.Lock(); defer c.mu.Unlock(); return c.cachedDept }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EmployeeUnit — cari unit kerja (branch/department) pegawai dari akun SSO.
// Dipakai IKU utk unit-scope operator (F8).
func (c *Client) EmployeeUnit(userID string) (*EmployeeInfo, error) {
	tok, err := c.getServiceTokenFn()
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequest(http.MethodGet, c.hcURL+"/api/employees?user_id="+url.QueryEscape(userID), nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hc employees: status %d", resp.StatusCode)
	}
	var env struct {
		Data []EmployeeInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	if len(env.Data) == 0 {
		return nil, nil // pegawai tidak ditemukan (belum di-onboard)
	}
	return &env.Data[0], nil
}

type EmployeeInfo struct {
	ID           string  `json:"id"`
	FullName     string  `json:"full_name"`
	BranchID     *string `json:"branch_id"`
	DepartmentID *string `json:"department_id"`
}
