package ticketstate

import "testing"

func TestTransisiLegal(t *testing.T) {
	legal := [][2]string{
		{Baru, MenungguApproval}, {Baru, Terbuka}, {Baru, Dibatalkan},
		{MenungguApproval, Terbuka}, {MenungguApproval, Ditolak}, {MenungguApproval, MenungguPemohon}, {MenungguApproval, Dibatalkan},
		{Terbuka, Dikerjakan}, {Terbuka, Dibatalkan},
		{Dikerjakan, MenungguPemohon}, {Dikerjakan, MenungguPihakLain}, {Dikerjakan, Selesai}, {Dikerjakan, Dibatalkan},
		{MenungguPemohon, Dikerjakan}, {MenungguPemohon, Ditutup},
		{MenungguPihakLain, Dikerjakan},
		{Selesai, Ditutup}, {Selesai, Dikerjakan}, // buka ulang
	}
	for _, tr := range legal {
		if !Can(tr[0], tr[1]) {
			t.Errorf("transisi %s -> %s seharusnya legal", tr[0], tr[1])
		}
	}
}

func TestTransisiIlegal(t *testing.T) {
	illegal := [][2]string{
		{Baru, Selesai}, {Baru, Dikerjakan}, {Baru, Ditutup},
		{Terbuka, Selesai},        // harus lewat Dikerjakan
		{MenungguPihakLain, Selesai}, // harus kembali ke Dikerjakan dulu
		{Ditutup, Dikerjakan},      // terminal: tidak dibuka ulang (§5.3)
		{Ditolak, Terbuka},
		{Dibatalkan, Baru},
		{Selesai, Baru},
	}
	for _, tr := range illegal {
		if Can(tr[0], tr[1]) {
			t.Errorf("transisi %s -> %s seharusnya ilegal", tr[0], tr[1])
		}
	}
}

func TestTerminal(t *testing.T) {
	for _, s := range []string{Ditutup, Ditolak, Dibatalkan} {
		if !IsTerminal(s) {
			t.Errorf("%s seharusnya terminal", s)
		}
	}
	for _, s := range []string{Baru, Terbuka, Dikerjakan, MenungguPemohon, MenungguPihakLain, Selesai, MenungguApproval} {
		if IsTerminal(s) {
			t.Errorf("%s seharusnya bukan terminal", s)
		}
	}
}

func TestJedaSLA(t *testing.T) {
	// SL-03: Menunggu Approval dan Menunggu Pemohon menjeda SLA.
	if !PausesSLA(MenungguApproval) || !PausesSLA(MenungguPemohon) {
		t.Error("menunggu_approval & menunggu_pemohon harus menjeda SLA")
	}
	// Menunggu Pihak Lain tetap berjalan (§5.3) kecuali diatur per layanan.
	if PausesSLA(MenungguPihakLain) || PausesSLA(Dikerjakan) || PausesSLA(Baru) || PausesSLA(Terbuka) {
		t.Error("status selain menunggu_approval/menunggu_pemohon tidak boleh menjeda SLA")
	}
}

func TestStatusTakDikenal(t *testing.T) {
	if Valid("haha") {
		t.Error("status tak dikenal harus invalid")
	}
	if Can("baru", "haha") || Can("haha", "terbuka") {
		t.Error("transisi dengan status tak dikenal harus ditolak")
	}
}
