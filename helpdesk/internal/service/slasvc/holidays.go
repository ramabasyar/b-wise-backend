package slasvc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// HolidayProvider — sinkron daftar hari libur dari Scheduling (calendar_events
// kind=holiday) untuk kalender SLA (BusinessCalendar.HolidaySource="scheduling").
// Cache in-memory TTL 1 jam; saat fetch gagal pakai cache lama (stale-ok).
type HolidayProvider struct {
	BaseURL      string // alamat Scheduling, mis. http://localhost:8085
	SSOURL       string // alamat SSO utk mint service token
	ClientID     string
	ClientSecret string
	client       *http.Client
	mu           sync.Mutex
	dates        []string
	fetchedAt    time.Time
	ttl          time.Duration
}

func NewHolidayProvider(baseURL, ssoURL, clientID, clientSecret string) *HolidayProvider {
	return &HolidayProvider{
		BaseURL: baseURL, SSOURL: ssoURL, ClientID: clientID, ClientSecret: clientSecret,
		client: &http.Client{Timeout: 10 * time.Second},
		ttl:    time.Hour,
	}
}

// Dates — daftar tanggal libur (YYYY-MM-DD).
func (p *HolidayProvider) Dates(ctx context.Context) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fetchedAt.IsZero() || time.Since(p.fetchedAt) > p.ttl {
		if dates, err := p.fetch(ctx); err == nil {
			p.dates, p.fetchedAt = dates, time.Now()
		} else if p.fetchedAt.IsZero() {
			return nil, err
		}
	}
	out := make([]string, len(p.dates))
	copy(out, p.dates)
	return out, nil
}

func (p *HolidayProvider) serviceToken(ctx context.Context) (string, error) {
	payload, _ := json.Marshal(map[string]string{
		"client_id":     p.ClientID,
		"client_secret": p.ClientSecret,
		"grant_type":    "client_credentials",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.SSOURL+"/api/oauth/token", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
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
	_ = json.Unmarshal(body, &r)
	if r.Data.AccessToken != "" {
		return r.Data.AccessToken, nil
	}
	if r.AccessToken != "" {
		return r.AccessToken, nil
	}
	return "", fmt.Errorf("token service kosong dari SSO")
}

func (p *HolidayProvider) fetch(ctx context.Context) ([]string, error) {
	tok, err := p.serviceToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.BaseURL+"/api/calendars/holidays", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scheduling holidays: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			Items []struct {
				Date string `json:"date"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(payload.Data.Items))
	for _, it := range payload.Data.Items {
		if len(it.Date) >= 10 {
			out = append(out, it.Date[:10])
		}
	}
	return out, nil
}

// AttachHolidays — pasang daftar libur ke kalender SLA.
func (c *Calendar) AttachHolidays(dates []string) { c.Holidays = dates }
