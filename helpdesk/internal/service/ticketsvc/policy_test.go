package ticketsvc

import (
	"testing"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		year int
		seq  int64
		want string
	}{
		{2026, 1, "PL-2026-000001"},
		{2026, 412, "PL-2026-000412"},
		{2027, 1234567, "PL-2027-1234567"}, // > 6 digit tidak dipangkas
	}
	for _, c := range cases {
		if got := FormatNumber(c.year, c.seq); got != c.want {
			t.Errorf("FormatNumber(%d, %d) = %s, ingin %s", c.year, c.seq, got, c.want)
		}
	}
}

func TestCanReopen(t *testing.T) {
	res := time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local)
	if !CanReopen(res, res.Add(6*24*time.Hour)) {
		t.Error("hari ke-6 masih dalam jendela 7 hari")
	}
	if CanReopen(res, res.Add(8*24*time.Hour)) {
		t.Error("hari ke-8 sudah lewat jendela")
	}
	if CanReopen(time.Time{}, time.Now()) {
		t.Error("resolvedAt nol tidak boleh reopen")
	}
}

func TestValidateTransition(t *testing.T) {
	now := time.Now()
	res := now.Add(-time.Hour)
	if err := ValidateTransition(ticketstate.Selesai, ticketstate.Dikerjakan, res, now); err != nil {
		t.Errorf("reopen dalam jendela harus lolos: %v", err)
	}
	if err := ValidateTransition(ticketstate.Selesai, ticketstate.Dikerjakan, now.Add(-8*24*time.Hour), now); err == nil {
		t.Error("reopen di luar jendela harus ditolak")
	}
	if err := ValidateTransition(ticketstate.Baru, ticketstate.Terbuka, time.Time{}, now); err != nil {
		t.Errorf("baru->terbuka harus lolos: %v", err)
	}
	if err := ValidateTransition(ticketstate.Baru, ticketstate.Selesai, time.Time{}, now); err == nil {
		t.Error("baru->selesai harus ditolak")
	}
	if err := ValidateTransition(ticketstate.Baru, "aneh", time.Time{}, now); err == nil {
		t.Error("status tak dikenal harus ditolak")
	}
}
