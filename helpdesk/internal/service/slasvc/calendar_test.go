package slasvc

import (
	"testing"
	"time"
)

func weekdaysCal() *Calendar {
	iv := []Interval{{Start: "08:00", End: "16:00"}}
	return &Calendar{
		Weekly: map[string][]Interval{"mon": iv, "tue": iv, "wed": iv, "thu": iv, "fri": iv},
		Timezone: "Asia/Jakarta",
	}
}

func at(t *testing.T, y int, m time.Month, d, h, min int) time.Time {
	t.Helper()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	return time.Date(y, m, d, h, min, 0, 0, loc)
}

// Senin 31 Okt 2026? tidak - 31 Okt 2026 = Sabtu. Pakai Senin 2 Nov 2026.
func TestAddDalamHariSama(t *testing.T) {
	c := weekdaysCal()
	from := at(t, 2026, time.November, 2, 9, 0) // Senin
	got := c.AddBusinessMinutes(from, 60)
	want := at(t, 2026, time.November, 2, 10, 0)
	if !got.Equal(want) {
		t.Errorf("harus %v, dapat %v", want, got)
	}
}

func TestAddLintasHari(t *testing.T) {
	c := weekdaysCal()
	from := at(t, 2026, time.November, 2, 15, 30) // Senin sore
	got := c.AddBusinessMinutes(from, 60)
	want := at(t, 2026, time.November, 3, 8, 30) // 30 mnt Senin + 30 mnt Selasa
	if !got.Equal(want) {
		t.Errorf("harus %v, dapat %v", want, got)
	}
}

func TestAddLewatiAkhirPekan(t *testing.T) {
	c := weekdaysCal()
	from := at(t, 2026, time.October, 30, 15, 0) // Jumat sore
	got := c.AddBusinessMinutes(from, 120)
	want := at(t, 2026, time.November, 2, 9, 0) // 60 Jumat + 60 Senin
	if !got.Equal(want) {
		t.Errorf("harus %v, dapat %v", want, got)
	}
}

func TestAddAwalMinggu(t *testing.T) {
	c := weekdaysCal()
	from := at(t, 2026, time.October, 31, 10, 0) // Sabtu
	got := c.AddBusinessMinutes(from, 30)
	want := at(t, 2026, time.November, 2, 8, 30)
	if !got.Equal(want) {
		t.Errorf("harus %v, dapat %v", want, got)
	}
}

func TestMenitAntara(t *testing.T) {
	c := weekdaysCal()
	if got := c.BusinessMinutesBetween(at(t, 2026, time.November, 2, 7, 0), at(t, 2026, time.November, 2, 17, 0)); got != 480 {
		t.Errorf("sehari penuh = 480, dapat %d", got)
	}
	if got := c.BusinessMinutesBetween(at(t, 2026, time.October, 30, 15, 0), at(t, 2026, time.November, 2, 9, 0)); got != 120 {
		t.Errorf("lintas akhir pekan = 120, dapat %d", got)
	}
}

func TestDueDates(t *testing.T) {
	c := weekdaysCal()
	tg, _ := ParseTargets(`{"normal":{"response_min":240,"resolution_min":2880}}`)
	created := at(t, 2026, time.October, 30, 15, 0) // Jumat 15:00
	dr, ds := DueDates(c, tg, "normal", created)
	if dr == nil || ds == nil {
		t.Fatal("due dates tidak boleh nil")
	}
	wantR := at(t, 2026, time.November, 2, 11, 0) // 60 Jumat + 180 Senin
	if !dr.Equal(wantR) {
		t.Errorf("respons harus %v, dapat %v", wantR, *dr)
	}
}
