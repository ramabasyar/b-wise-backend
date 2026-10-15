package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// QueueConfig - koneksi REST API MikroTik (RouterOS v7) untuk simple queue.
type QueueConfig struct {
	BaseURL            string // mis. https://192.168.254.1
	User               string
	Pass               string
	InsecureSkipVerify bool
	DefaultUp          string        // fallback upload, contoh "10M"
	DefaultDown        string        // fallback download, contoh "20M"
	Interval           time.Duration // default 20 detik
}

// QueueService menyinkronkan simple queue MikroTik dengan sesi WiFi aktif:
// satu queue per IP klien (target <ip>/32); rate = override user > profil > default.
// User suspended yang sesinya masih hidup (AP UniFi tidak merespons CoA)
// di-throttle ke 1k/1k sebagai containment sampai sesi berakhir / auth berikutnya ditolak.
type QueueService struct {
	policy *PolicyService
	cfg    QueueConfig
	client *http.Client
	logf   func(format string, args ...interface{})
}

func NewQueueService(p *PolicyService, cfg QueueConfig, logf func(string, ...interface{})) *QueueService {
	if cfg.Interval <= 0 {
		cfg.Interval = 20 * time.Second
	}
	if cfg.DefaultUp == "" {
		cfg.DefaultUp = "10M"
	}
	if cfg.DefaultDown == "" {
		cfg.DefaultDown = "20M"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}}
	return &QueueService{
		policy: p,
		cfg:    cfg,
		client: &http.Client{Timeout: 15 * time.Second, Transport: tr},
		logf:   logf,
	}
}

type mtQueue struct {
	ID       string `json:".id"`
	Name     string `json:"name"`
	Target   string `json:"target"`
	MaxLimit string `json:"max-limit"`
	Comment  string `json:"comment"`
}

type mtLease struct {
	Address    string `json:"address"`
	MACAddress string `json:"mac-address"`
}

func (q *QueueService) do(method, path string, body interface{}) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, q.cfg.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(q.cfg.User, q.cfg.Pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mikrotik %s %s: HTTP %d: %s", method, path, resp.StatusCode, truncStr(string(data), 200))
	}
	return data, nil
}

func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// normalizeMAC membuang pemisah dan menyeragamkan ke huruf kecil hex.
func normalizeMAC(m string) string {
	var b strings.Builder
	for _, c := range m {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			b.WriteRune(c)
		}
	}
	return strings.ToLower(b.String())
}

// colonMAC mengubah 3ea832eb4516 menjadi 3E:A8:32:EB:45:16 (format lease MikroTik).
func colonMAC(m string) string {
	hex := normalizeMAC(m)
	if len(hex) != 12 {
		return ""
	}
	parts := make([]string, 0, 6)
	for i := 0; i < 12; i += 2 {
		parts = append(parts, strings.ToUpper(hex[i:i+2]))
	}
	return strings.Join(parts, ":")
}

func queueNameForIP(ip string) string {
	return "airlink-" + strings.ReplaceAll(stripCIDR(ip), ".", "-")
}

// Run menjalankan reconciler sampai ctx dibatalkan.
func (q *QueueService) Run(ctx context.Context) {
	q.logf("reconciler queue MikroTik mulai (interval %s)", q.cfg.Interval)
	t := time.NewTicker(q.cfg.Interval)
	defer t.Stop()
	for {
		q.reconcileOnce()
		select {
		case <-ctx.Done():
			q.logf("reconciler queue MikroTik berhenti")
			return
		case <-t.C:
		}
	}
}

type desiredQueue struct {
	Username string
	Up, Down string
}

func (q *QueueService) reconcileOnce() {
	defer func() {
		if r := recover(); r != nil {
			q.logf("panic reconcile: %v", r)
		}
	}()
	refs, err := q.policy.ActiveSessionRefs()
	if err != nil {
		q.logf("radacct: %v", err)
		return
	}
	rates, err := q.policy.EffectiveRates(q.cfg.DefaultUp, q.cfg.DefaultDown)
	if err != nil {
		q.logf("policy rates: %v", err)
		return
	}
	leaseIP := map[string]string{} // MAC ternormalisasi -> IP
	if raw, err := q.do("GET", "/rest/ip/dhcp-server/lease", nil); err == nil {
		var leases []mtLease
		if json.Unmarshal(raw, &leases) == nil {
			for _, l := range leases {
				leaseIP[normalizeMAC(l.MACAddress)] = stripCIDR(l.Address)
			}
		}
	} else {
		q.logf("dhcp lease: %v", err)
	}
	desired := map[string]desiredQueue{} // IP -> limit
	for _, r := range refs {
		ip := stripCIDR(r.FramedIPAddress)
		if ip == "" {
			ip = leaseIP[normalizeMAC(r.CallingStationID)]
		}
		if ip == "" {
			continue
		}
		info := rates[r.Username]
		up, down := info.Up, info.Down
		if info.Suspended {
			up, down = "1k", "1k"
		}
		desired[ip] = desiredQueue{Username: r.Username, Up: up, Down: down}
	}
	var existing []mtQueue
	raw, err := q.do("GET", "/rest/queue/simple", nil)
	if err != nil {
		q.logf("queue list: %v", err)
		return
	}
	if json.Unmarshal(raw, &existing) != nil {
		q.logf("queue list: respons tidak valid")
		return
	}
	current := map[string]mtQueue{} // IP -> queue milik airlink
	for _, eq := range existing {
		if strings.HasPrefix(eq.Comment, "airlink:") {
			current[stripCIDR(eq.Target)] = eq
		}
	}
	for ip, d := range desired {
		limit := d.Up + "/" + d.Down
		cur, ok := current[ip]
		if !ok {
			body := map[string]string{
				"name":      queueNameForIP(ip),
				"target":    ip + "/32",
				"max-limit": limit,
				"comment":   "airlink:" + d.Username,
			}
			if _, err := q.do("POST", "/rest/queue/simple", body); err != nil {
				q.logf("queue create %s: %v", ip, err)
			}
			continue
		}
		if cur.MaxLimit != limit {
			body := map[string]string{"max-limit": limit}
			if _, err := q.do("PUT", "/rest/queue/simple/"+strings.ReplaceAll(cur.ID, "/", "%2F"), body); err != nil {
				q.logf("queue update %s: %v", ip, err)
			}
		}
	}
	for ip, eq := range current {
		if _, ok := desired[ip]; !ok {
			if _, err := q.do("DELETE", "/rest/queue/simple/"+strings.ReplaceAll(eq.ID, "/", "%2F"), nil); err != nil {
				q.logf("queue delete %s: %v", ip, err)
			}
		}
	}
}
