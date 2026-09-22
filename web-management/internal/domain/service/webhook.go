package service

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// PublishEvent — payload webhook BWM (stub F0; dikembangkan F1 bersama web konsumer).
type PublishEvent struct {
	Event  string    `json:"event"` // publish | archive
	Entity string    `json:"entity"` // post | event | page | banner
	ID     string    `json:"id"`
	Slug   string    `json:"slug,omitempty"`
	At     time.Time `json:"at"`
}

// FireWebhooks — best-effort POST ke semua URL (fire-and-forget, 5s timeout).
func FireWebhooks(urls []string, ev PublishEvent) {
	if len(urls) == 0 {
		return
	}
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	for _, u := range urls {
		go func(u string) {
			client := http.Client{Timeout: 5 * time.Second}
			resp, err := client.Post(u, "application/json", bytes.NewReader(body))
			if err != nil {
				log.Printf("[webhook] %s gagal: %v", u, err)
				return
			}
			_ = resp.Body.Close()
			log.Printf("[webhook] %s -> %d (%s %s)", u, resp.StatusCode, ev.Entity, ev.Slug)
		}(u)
	}
}
