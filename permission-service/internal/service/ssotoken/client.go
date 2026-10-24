package ssotoken

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Client — mengambil & men-cache service token dari SSO (client credentials).
// Dipakai permission-service untuk memanggil internal API service lain
// (mis. HC /api/org/resolve-approver — audit M5) tanpa mem-forward JWT user.
type Client struct {
	baseURL     string
	clientID    string
	secret      string
	httpClient  *http.Client
	mu          sync.Mutex
	token       string
	tokenExpiry time.Time
}

func New(baseURL, clientID, clientSecret string) *Client {
	return &Client{
		baseURL:    baseURL,
		clientID:   clientID,
		secret:     clientSecret,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Token — service token valid (di-cache dengan buffer 60 detik).
func (c *Client) Token() (string, error) {
	if c.clientID == "" || c.secret == "" {
		return "", fmt.Errorf("service client SSO belum dikonfigurasi (SERVICE_CLIENT_ID/SERVICE_CLIENT_SECRET)")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}
	body := fmt.Sprintf(`{"client_id":"%s","client_secret":"%s","grant_type":"client_credentials"}`,
		c.clientID, c.secret)
	resp, err := c.httpClient.Post(c.baseURL+"/api/oauth/token", "application/json", bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("SSO token request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result struct {
		Data struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		} `json:"data"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("gagal parse respons token SSO (%s): %w", string(respBody[:smin(len(respBody), 120)]), err)
	}
	token, expiry := result.Data.AccessToken, result.Data.ExpiresIn
	if token == "" {
		token, expiry = result.AccessToken, result.ExpiresIn
	}
	if token == "" {
		return "", fmt.Errorf("access token kosong dari SSO (HTTP %d)", resp.StatusCode)
	}
	if expiry <= 0 {
		expiry = 900
	}
	c.token = token
	c.tokenExpiry = time.Now().Add(time.Duration(expiry-60) * time.Second)
	return token, nil
}

func smin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
