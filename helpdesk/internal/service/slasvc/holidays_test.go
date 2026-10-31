package slasvc

import (
	"testing"
	"time"
)

// TestAddBusinessMinutesSkipHoliday — libur dari Scheduling harus dilewati (integrasi batch 5).
// Kamis 24 Des 15:00 +120 mnt kerja: 60 mnt habis Kamis (15:00-16:00); Jumat 25 libur;
// Sabtu-Minggu libur mingguan; sisa 60 mnt → Senin 28 Des 09:00.
func TestAddBusinessMinutesSkipHoliday(t *testing.T) {
	iv := []Interval{{Start: "08:00", End: "16:00"}}
	c := &Calendar{
		Weekly:   map[string][]Interval{"mon": iv, "tue": iv, "wed": iv, "thu": iv, "fri": iv},
		Timezone: "Asia/Jakarta",
		Holidays: []string{"2026-12-25"},
	}
	wib, _ := time.LoadLocation("Asia/Jakarta")
	from := time.Date(2026, 12, 24, 15, 0, 0, 0, wib)
	got := c.AddBusinessMinutes(from, 120)
	want := time.Date(2026, 12, 28, 9, 0, 0, 0, wib)
	if !got.Equal(want) {
		t.Fatalf("hasil %v, ingin %v (24 Des 15:00 +120mnt → lewati libur 25 & akhir pekan → Senin 09:00)", got, want)
	}
}
