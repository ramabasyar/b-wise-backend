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

// chatModel - nama model obrolan dari env (default qwen2.5:3b-instruct utk dev Mac).
func (c *Client) chatModel() string {
	if m := os.Getenv("AI_CHAT_MODEL"); m != "" {
		return m
	}
	return "qwen2.5:3b-instruct"
}

// Chat - satu putaran obrolan (non-stream). System opsional.
// Dekode via map (tanpa struct tag) supaya bebas jebakan kutip lintas-lapis.
func (c *Client) Chat(system, user string) (string, error) {
	msgs := []map[string]string{}
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": user})
	body, _ := json.Marshal(map[string]any{"model": c.chatModel(), "messages": msgs, "stream": false})
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai chat: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("ai chat: HTTP %d: %s", res.StatusCode, string(b)[:min(200, len(b))])
	}
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	msg, _ := out["message"].(map[string]any)
	content, _ := msg["content"].(string)
	return content, nil
}
