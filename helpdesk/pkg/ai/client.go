package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client — adapter embedding lintas provider (KB-06/AG-13/OT-06, F4).
// Dev: ollama lokal (AI_BASE_URL=http://localhost:11434, model bge-m3).
// Produksi nanti: server AI kampus (altos) atau penyedia lain — kontrak sama.
type Client struct {
	baseURL string
	model   string
	http    *http.Client
}

// New dari env: AI_BASE_URL (default http://localhost:11434) + AI_EMBED_MODEL (default bge-m3).
// URL kosong = AI mati (fitur semantik fallback ke kata-kunci).
func New() *Client {
	u := os.Getenv("AI_BASE_URL")
	if u == "" {
		u = "http://localhost:11434"
	}
	m := os.Getenv("AI_EMBED_MODEL")
	if m == "" {
		m = "bge-m3"
	}
	return &Client{baseURL: u, model: m, http: &http.Client{Timeout: 30 * time.Second}}
}

// Enabled — false bila AI_DISABLED=1 (fallback pencarian kata-kunci).
func (c *Client) Enabled() bool {
	return os.Getenv("AI_DISABLED") != "1"
}

// Embed — satu teks → vektor (dimensi mengikuti model; bge-m3 = 1024).
func (c *Client) Embed(text string) ([]float32, error) {
	vecs, err := c.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) != 1 {
		return nil, fmt.Errorf("embedding: mengharapkan 1 vektor, dapat %d", len(vecs))
	}
	return vecs[0], nil
}

// EmbedBatch — banyak teks sekali panggil (ollama /api/embed).
func (c *Client) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body, _ := json.Marshal(map[string]any{"model": c.model, "input": texts})
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai embed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ai embed: HTTP %d: %s", res.StatusCode, string(b)[:min(200, len(b))])
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Embeddings, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
