// Package slasvc - mesin SLA Pusat Layanan (blueprint SL-01..SL-07, 9.5).
package slasvc

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Interval - satu rentang jam kerja HH:MM.
type Interval struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Calendar - kalender jam layanan (business_calendars.weekly_hours jsonb).
type Calendar struct {
	Weekly   map[string][]Interval `json:"weekly_hours"`
	Timezone string               `json:"timezone"`
	Holidays []string             `json:"-"`
}

var dayKeys = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// Loc - *time.Location kalender (fallback WIB).
func (c *Calendar) Loc() *time.Location {
	tz := c.Timezone
	if tz == "" {
		tz = "Asia/Jakarta"
	}
	if loc, err := time.LoadLocation(tz); err == nil {
		return loc
	}
	return time.FixedZone("WIB", 7*3600)
}

func parseHM(s string) (int, int) {
	parts := strings.Split(s, ":")
	h, m := 0, 0
	if len(parts) >= 1 {
		fmt.Sscanf(parts[0], "%d", &h)
	}
	if len(parts) >= 2 {
		fmt.Sscanf(parts[1], "%d", &m)
	}
	return h, m
}

func isHoliday(c *Calendar, d time.Time) bool {
	key := d.Format("2006-01-02")
	for _, h := range c.Holidays {
		if h == key {
			return true
		}
	}
	return false
}

// AddBusinessMinutes - maju 'minutes' menit jam kerja dari 'from'.
func (c *Calendar) AddBusinessMinutes(from time.Time, minutes int) time.Time {
	loc := c.Loc()
	t := from.In(loc)
	remaining := minutes
	for i := 0; i < 60; i++ {
		dayKey := dayKeys[int(t.Weekday())]
		for _, iv := range c.Weekly[dayKey] {
			sh, sm := parseHM(iv.Start)
			eh, em := parseHM(iv.End)
			start := time.Date(t.Year(), t.Month(), t.Day(), sh, sm, 0, 0, loc)
			end := time.Date(t.Year(), t.Month(), t.Day(), eh, em, 0, 0, loc)
			if end.Before(start) || !t.Before(end) {
				continue
			}
			cursor := t
			if cursor.Before(start) {
				cursor = start
			}
			avail := int(end.Sub(cursor).Minutes())
			if avail <= 0 {
				continue
			}
			if avail >= remaining {
				return cursor.Add(time.Duration(remaining) * time.Minute)
			}
			remaining -= avail
			t = end
		}
		next := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		if isHoliday(c, next) {
			t = next
			continue
		}
		t = next
	}
	return t
}

// BusinessMinutesBetween - menit jam kerja antara dua waktu (geser due saat jeda).
func (c *Calendar) BusinessMinutesBetween(from, to time.Time) int {
	if !to.After(from) {
		return 0
	}
	loc := c.Loc()
	f := from.In(loc)
	total := 0
	for i := 0; i < 120; i++ {
		dayKey := dayKeys[int(f.Weekday())]
		for _, iv := range c.Weekly[dayKey] {
			sh, sm := parseHM(iv.Start)
			eh, em := parseHM(iv.End)
			start := time.Date(f.Year(), f.Month(), f.Day(), sh, sm, 0, 0, loc)
			end := time.Date(f.Year(), f.Month(), f.Day(), eh, em, 0, 0, loc)
			if end.Before(start) {
				continue
			}
			lo, hi := start, end
			if f.After(lo) {
				lo = f
			}
			if to.Before(hi) {
				hi = to
			}
			if hi.After(lo) {
				total += int(hi.Sub(lo).Minutes())
			}
		}
		next := time.Date(f.Year(), f.Month(), f.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		if !next.Before(to.In(loc)) {
			break
		}
		f = next
	}
	return total
}

// ParseCalendar - dari kolom jsonb business_calendars.
func ParseCalendar(weeklyJSON, tz string) (*Calendar, error) {
	c := &Calendar{Weekly: map[string][]Interval{}, Timezone: tz}
	if weeklyJSON != "" {
		if err := json.Unmarshal([]byte(weeklyJSON), &c.Weekly); err != nil {
			return nil, err
		}
	}
	return c, nil
}
