// Package ticketsvc — logika inti tiket Pusat Layanan (F1; blueprint 5, 11.2).
package ticketsvc

import (
	"fmt"
	"time"

	"github.com/rama/b-wise/helpdesk/internal/domain/ticketstate"
)

// ReopenWindow — batas buka ulang tiket Selesai (5.3: 7 hari).
const ReopenWindow = 7 * 24 * time.Hour

// FormatNumber merender nomor tiket PL-YYYY-NNNNNN (6 digit).
func FormatNumber(year int, seq int64) string {
	return fmt.Sprintf("PL-%d-%06d", year, seq)
}

// CanReopen — tiket Selesai boleh dibuka ulang dalam ReopenWindow.
func CanReopen(resolvedAt time.Time, now time.Time) bool {
	if resolvedAt.IsZero() {
		return false
	}
	return now.Sub(resolvedAt) <= ReopenWindow
}

// ValidateTransition — guard transisi + aturan khusus (jendela buka ulang).
func ValidateTransition(from, to string, resolvedAt, now time.Time) error {
	if !ticketstate.Valid(to) {
		return fmt.Errorf("status tujuan tidak dikenal: %s", to)
	}
	if from == ticketstate.Selesai && to == ticketstate.Dikerjakan {
		if !CanReopen(resolvedAt, now) {
			return fmt.Errorf("buka ulang hanya boleh dalam 7 hari seelah Selesai")
		}
		return nil
	}
	if !ticketstate.Can(from, to) {
		return fmt.Errorf("transisi %s -> %s tidak diizinkan", from, to)
	}
	return nil
}
