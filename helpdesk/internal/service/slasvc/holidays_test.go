package slasvc

import (
	"testing"
	"time"
)

// TestAddBusinessMinutesSkipHoliday — libur dari Scheduling harus dilewati (integrasi batch 5).
func TestAddBusinessMinutesSkipHoliday(t *testing.T) {
	iv := []Interval{{Start: "08:00", End: "16:00"}}
	c := &Calendar{
		Weekly:   map[string][]Interval{"mon": iv, "tue": iv, "wed": iv, "thu": iv, "fri": iv},
		Timezone: "Asia/Jakarta",
		Holidays: []string{"2026-12-25"},
	}
	wib, _ := time.LoadLocation("Asia/Jakarta")
	// Kamis 24 Des 2026 15:00 WIB + 60 menit kerja → Jumat (25 libur, dilewati) tidak, langsung Sabtu? Sabtu libur mingguan → Senin 28 Des 09:00.
	from := time.Date(2026, 12, 24, 15, 0, 0, 0, wib)
	got := c.AddBusinessMinutes(from, 60)
	want := time.Date(2026, 12, 28, 9, 0, 0, 0, wib)
	if !got.Equal(want) {
		t.Fatalf("hasil %v, ingin %v (24 Des 15:00 +60mnt → lewati 25 libur & akhir pekan → Senin 09:00)", got, want)
	}
}
