// Package ssoclient — minimal SSO client credentials client untuk service-to-service auth.
package ssoclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type Client struct {
	ssoURL, clientID, secret string
	http                     *http.Client
	mu                       sync.Mutex
	token                    string
	expiry                   time.Time
}

func New(ssoURL, clientID, secret string) *Client {
	return &Client{ssoURL: ssoURL, clientID: clientID, secret: secret, http: &http.Client{Timeout: 10 * time.Second}}
}

// GetServiceToken — cached, refresh 60s sebelum expiry.
func (c *Client) GetServiceToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expiry) {
		return c.token, nil
	}
	body, _ := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     c.clientID,
		"client_secret": c.secret,
	})
	resp, err := c.http.Post(c.ssoURL+"/api/oauth/token", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var r struct {
		Data struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("parse SSO token resp: %w", err)
	}
	tok, exp := r.Data.AccessToken, r.Data.ExpiresIn
	if tok == "" {
		tok, exp = r.AccessToken, r.ExpiresIn
	}
	if tok == "" {
		return "", fmt.Errorf("empty service token: %s", string(raw[:min(len(raw), 120)]))
	}
	if exp <= 0 {
		exp = 900
	}
	c.token = tok
	c.expiry = time.Now().Add(time.Duration(exp-60) * time.Second)
	return tok, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
