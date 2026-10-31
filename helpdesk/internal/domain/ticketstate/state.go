// Package ticketstate — state machine status tiket Pusat Layanan (blueprint §5.3).
// Semua perubahan status lewat Can(); transisi di luar peta = ditolak.
package ticketstate

// Status tiket.
const (
	Baru             = "baru"
	MenungguApproval = "menunggu_approval"
	Terbuka          = "terbuka"
	Dikerjakan       = "dikerjakan"
	MenungguPemohon  = "menunggu_pemohon"
	MenungguPihakLain = "menunggu_pihak_lain"
	Selesai          = "selesai"
	Ditutup          = "ditutup"
	Ditolak          = "ditolak"
	Dibatalkan       = "dibatalkan"
)

// transitions — peta transisi legal (from -> set of to).
var transitions = map[string]map[string]bool{
	Baru: {MenungguApproval: true, Terbuka: true, Dibatalkan: true},
	MenungguApproval: {Terbuka: true, Ditolak: true, MenungguPemohon: true, Dibatalkan: true},
	Terbuka:          {Dikerjakan: true, Dibatalkan: true},
	Dikerjakan:       {MenungguPemohon: true, MenungguPihakLain: true, Selesai: true, Dibatalkan: true},
	MenungguPemohon:  {Dikerjakan: true, Ditutup: true},
	MenungguPihakLain: {Dikerjakan: true},
	Selesai:          {Ditutup: true, Dikerjakan: true}, // buka ulang <= 7 hari (§5.3)
}

// terminal — status akhir; tidak boleh keluar lagi.
var terminal = map[string]bool{Ditutup: true, Ditolak: true, Dibatalkan: true}

// pauseSLA — status yang menjeda jam SLA pemohon (SL-03).
var pauseSLA = map[string]bool{MenungguApproval: true, MenungguPemohon: true}

// Valid melaporkan apakah string adalah status yang dikenal.
func Valid(s string) bool {
	switch s {
	case Baru, MenungguApproval, Terbuka, Dikerjakan, MenungguPemohon, MenungguPihakLain, Selesai, Ditutup, Ditolak, Dibatalkan:
		return true
	}
	return false
}

// Can melaporkan apakah transisi from -> to legal.
func Can(from, to string) bool {
	if !Valid(from) || !Valid(to) {
		return false
	}
	return transitions[from][to]
}

// IsTerminal melaporkan apakah status status akhir.
func IsTerminal(s string) bool { return terminal[s] }

// PausesSLA melaporkan apakah status menjeda jam SLA pemohon.
func PausesSLA(s string) bool { return pauseSLA[s] }

// Initial mengembalikan status awal tiket baru.
func Initial() string { return Baru }
