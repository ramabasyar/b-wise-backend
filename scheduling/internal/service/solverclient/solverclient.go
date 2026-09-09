package solverclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ==================== SOLVER CLIENT (F1) ====================
// Memanggil sidecar Python (FastAPI + OR-Tools) via streaming NDJSON.
// Setiap baris = event progress; baris terakhir = hasil.

type SolverClient struct {
	BaseURL string
	HTTP    *http.Client
}

func NewSolverClient(baseURL string) *SolverClient {
	return &SolverClient{BaseURL: baseURL, HTTP: &http.Client{Timeout: 15 * time.Minute}}
}

// SolverEvent — satu baris NDJSON dari sidecar.
type SolverEvent struct {
	Phase       string           `json:"phase"`
	Progress    int              `json:"progress"`
	Message     string           `json:"message"`
	Elapsed     *float64         `json:"elapsed,omitempty"`
	Stats       map[string]any   `json:"stats,omitempty"`
	Assignments []map[string]any `json:"assignments,omitempty"`
}

// SolverPayload — model yang dikirim ke sidecar.
type SolverPayload struct {
	JobID            string             `json:"job_id"`
	Sessions         []SolverSession    `json:"sessions"`
	Rooms            []SolverRoom       `json:"rooms"`
	RoomTypes        []SolverRoomType   `json:"room_types"`
	Slots            []SolverSlot       `json:"slots"`
	LecturerBlocks   []SolverBlock      `json:"lecturer_blocks"`
	SolveConfig      *SolverConfig      `json:"solve_config,omitempty"` // bobot soft constraint (default 5/2/1)
	TimeLimitSeconds int                `json:"time_limit_seconds"`
	Locked           []map[string]any   `json:"locked"`
}

// SolverConfig — bobot soft constraint dinamis (tabel solve_configs).
type SolverConfig struct {
	SpreadWeight    int `json:"spread_weight"`
	RoomWasteWeight int `json:"room_waste_weight"`
	LastSlotWeight  int `json:"last_slot_weight"`
}

// SolverRoomType — kamus tipe ruang dinamis; solver memakai flags utk
// menentukan ruang eligible (data-driven, menggantikan mapping hardcode).
type SolverRoomType struct {
	Code        string `json:"code"`
	ForTheory   bool   `json:"for_theory"`
	ForPractice bool   `json:"for_practice"`
}

type SolverSession struct {
	Key           string `json:"key"`
	OfferingID    string `json:"offering_id"`
	CourseCode    string `json:"course_code"`
	CourseType    string `json:"course_type"`
	RoomNeed      string `json:"room_need"` // theory|practice|any|none — hasil resolve kamus course_types
	DurationSlots int    `json:"duration_slots"`                 // fallback bila duration_minutes kosong
	DurationMinutes int  `json:"duration_minutes,omitempty"` // prioritas: blok slot berurutan span ≥ menit
	RoomType      string `json:"room_type"`
	GroupID       string `json:"group_id"`
	GroupSize     int    `json:"group_size"`
	LecturerID    string `json:"lecturer_id"`
}
type SolverRoom struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Type     string `json:"type"`
	Capacity int    `json:"capacity"`
}
type SolverSlot struct {
	ID        string `json:"id"`
	Day       int    `json:"day"`
	Order     int    `json:"order"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}
type SolverBlock struct {
	LecturerID string `json:"lecturer_id"`
	Day        int    `json:"day"`
}

// StreamSolve — POST /solve, panggil onEvent per baris NDJSON.
// Return error kalau stream gagal; hasil akhir dikirim via onEvent(phase=done/failed).
func (c *SolverClient) StreamSolve(ctx context.Context, p SolverPayload, onEvent func(SolverEvent) error) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/solve", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("solver sidecar tidak terjangkau: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("solver error HTTP %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024) // baris assignments besar
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev SolverEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if err := onEvent(ev); err != nil {
			return err // dibatalkan oleh caller
		}
	}
	return sc.Err()
}
