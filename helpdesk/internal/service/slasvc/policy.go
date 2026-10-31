package slasvc

import (
	"encoding/json"
	"time"
)

// Target - batas waktu satu prioritas (menit).
type Target struct {
	ResponseMin   int `json:"response_min"`
	ResolutionMin int `json:"resolution_min"`
}

// ParseTargets - dari sla_policies.targets jsonb.
func ParseTargets(js string) (map[string]Target, error) {
	m := map[string]Target{}
	if js == "" {
		return m, nil
	}
	err := json.Unmarshal([]byte(js), &m)
	return m, err
}

// DueDates - hitung due respons & penyelesaian dari kalender bisnis.
func DueDates(cal *Calendar, targets map[string]Target, priority string, createdAt time.Time) (dueResponse, dueResolution *time.Time) {
	tg, ok := targets[priority]
	if !ok {
		tg, ok = targets["normal"]
	}
	if !ok {
		return nil, nil
	}
	dr := cal.AddBusinessMinutes(createdAt, tg.ResponseMin)
	ds := cal.AddBusinessMinutes(createdAt, tg.ResolutionMin)
	return &dr, &ds
}
