// Package notifyclient relays domain outboxes to notify without equating acceptance with delivery.
package notifyclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type Message struct {
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	UserID    string `json:"user_id"`
	Topic     string `json:"topic"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Payload   string `json:"payload"`
}
type Result struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
}
type Client struct {
	base, sso, id, secret, token string
	until                        time.Time
	mu                           sync.Mutex
	http                         *http.Client
}

func New() *Client {
	return &Client{base: strings.TrimRight(os.Getenv("NOTIFY_URL"), "/"), sso: strings.TrimRight(os.Getenv("SSO_URL"), "/"), id: os.Getenv("SERVICE_CLIENT_ID"), secret: os.Getenv("SERVICE_CLIENT_SECRET"), http: &http.Client{Timeout: 10 * time.Second}}
}
func (c *Client) Enabled() bool { return c.base != "" }
func (c *Client) accessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.until) {
		return c.token, nil
	}
	b, _ := json.Marshal(map[string]string{"client_id": c.id, "client_secret": c.secret, "grant_type": "client_credentials"})
	resp, err := c.http.Post(c.sso+"/api/oauth/token", "application/json", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("SSO HTTP %d", resp.StatusCode)
	}
	var r struct {
		AccessToken string `json:"access_token"`
		Data        struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return "", err
	}
	c.token = r.AccessToken
	if c.token == "" {
		c.token = r.Data.AccessToken
	}
	if c.token == "" {
		return "", fmt.Errorf("SSO tidak mengembalikan service token")
	}
	c.until = time.Now().Add(4 * time.Minute)
	return c.token, nil
}

// Replaying the same domain row is safe even after a lost HTTP response.
func (c *Client) Relay(key string, msg Message) (Result, error) {
	var out Result
	tk, err := c.accessToken()
	if err != nil {
		return out, err
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequest("POST", c.base+"/api/internal/notifications", bytes.NewReader(b))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+tk)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("notify HTTP %d", resp.StatusCode)
	}
	var r struct {
		Data Result `json:"data"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return out, err
	}
	if r.Data.ID == "" || r.Data.Status == "" {
		return out, fmt.Errorf("respons notify tidak lengkap")
	}
	return r.Data, nil
}
