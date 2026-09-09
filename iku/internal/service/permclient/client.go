package permclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// permclient — F8: cek permission user di permission-service (service token),
// dipakai IKU untuk menentukan unit-scope operator (punya achievements.write
// TAPI TIDAK achievements.workflow → operator unit, view terkunci unit-nya).

type Client struct {
	baseURL      string
	serviceToken func() (string, error)

	mu        sync.Mutex
	serviceID string
	idFetched time.Time
	permCache map[string]permEntry // key: uid|perm
}

type permEntry struct {
	has bool
	at  time.Time
}

const cacheTTL = 5 * time.Minute

func New(baseURL string, tokenFn func() (string, error)) *Client {
	return &Client{baseURL: baseURL, serviceToken: tokenFn, permCache: map[string]permEntry{}}
}

func (c *Client) get(path string, out interface{}) error {
	tok, err := c.serviceToken()
	if err != nil {
		return fmt.Errorf("service token: %w", err)
	}
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("perm service %s: status %d", path, resp.StatusCode)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return err
	}
	if len(envelope.Data) == 0 {
		return fmt.Errorf("perm service %s: kosong", path)
	}
	return json.Unmarshal(envelope.Data, out)
}

// serviceID resolves nama "iku" → UUID (cache).
func (c *Client) ensureServiceID() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.serviceID != "" && time.Since(c.idFetched) < time.Hour {
		return c.serviceID, nil
	}
	var services []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.get("/api/services", &services); err != nil {
		return "", err
	}
	for _, s := range services {
		if s.Name == "iku" {
			c.serviceID, c.idFetched = s.ID, time.Now()
			return c.serviceID, nil
		}
	}
	return "", fmt.Errorf("service 'iku' tidak terdaftar")
}

// HasPermission — cache 5 menit per (user, perm).
func (c *Client) HasPermission(userID, perm string) bool {
	if userID == "" {
		return false
	}
	key := userID + "|" + perm
	c.mu.Lock()
	if e, ok := c.permCache[key]; ok && time.Since(e.at) < cacheTTL {
		c.mu.Unlock()
		return e.has
	}
	c.mu.Unlock()

	svcID, err := c.ensureServiceID()
	if err != nil {
		return false // fail-closed utk scoping? — kita fail-OPEN di caller dgn log
	}
	var res struct {
		HasPermission bool `json:"has_permission"`
	}
	if err := c.get(fmt.Sprintf("/api/users/%s/permissions/check?service_id=%s&permission=%s", userID, svcID, perm), &res); err != nil {
		return false
	}
	c.mu.Lock()
	c.permCache[key] = permEntry{has: res.HasPermission, at: time.Now()}
	c.mu.Unlock()
	return res.HasPermission
}

// IsOperator — punya write capaian tapi TIDAK workflow (reviewer/pimpinan = false).
func (c *Client) IsOperator(userID string) bool {
	if userID == "" {
		return false
	}
	return c.HasPermission(userID, "achievements.write") && !c.HasPermission(userID, "achievements.workflow")
}
