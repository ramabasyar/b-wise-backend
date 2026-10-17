package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ebfe/scard"
)

const version = "0.2.2"

const defaultKioskURL = "https://stg-bwise.binawan.ac.id/kiosk/library"

var (
	flagReader    = flag.String("reader", "ACR122", "substring nama reader yang dipakai (case-insensitive)")
	flagList      = flag.Bool("list", false, "tampilkan daftar smart card reader lalu keluar")
	flagDryRun    = flag.Bool("dry-run", false, "baca & tampilkan UID tanpa mengetik (mode uji)")
	flagDelay     = flag.Int("delay", 8, "jeda antar ketikan (ms)")
	flagNoBeep    = flag.Bool("no-beep", false, "tanpa beep saat kartu terbaca")
	flagNoEnter   = flag.Bool("no-enter", false, "jangan tekan Enter setelah UID diketik")
	flagDebug     = flag.Bool("debug", false, "log verbose (termasuk ATR kartu)")
	flagURL       = flag.String("url", defaultKioskURL, "URL halaman kiosk yang dibuka otomatis")
	flagNoBrowser = flag.Bool("no-browser", false, "jangan buka browser otomatis saat start")
)

func main() {
	flag.Parse()
	log.SetFlags(log.Ltime)
	fmt.Printf("library_visitor v%s - ACR122U -> keyboard wedge\n", version)
	fmt.Println("Ketik 'library_visitor.exe -h' untuk opsi.")

	if !singleInstance() {
		log.Fatalf("library_visitor sudah berjalan (instance lain masih aktif) - tutup dulu.")
	}
	fmt.Println()

	ctx, err := scard.EstablishContext()
	if err != nil {
		log.Fatalf("PC/SC context gagal: %v", err)
	}
	defer ctx.Release()

	readers, err := ctx.ListReaders()
	if err != nil {
		if errors.Is(err, scard.ErrNoService) || errors.Is(err, scard.ErrServiceStopped) {
			log.Fatalf("Smart Card service Windows tidak jalan - cek services.msc (Smart Card)")
		}
		log.Fatalf("list reader gagal: %v", err)
	}
	if len(readers) == 0 {
		log.Fatalf("tidak ada smart card reader terdeteksi - cek Device Manager (driver ACR122U)")
	}
	if *flagList {
		fmt.Println("Smart card reader terdeteksi:")
		for i, r := range readers {
			fmt.Printf("  [%d] %s\n", i, r)
		}
		return
	}

	name := pickReader(readers, *flagReader)
	fmt.Printf("Reader     : %s\n", name)
	fmt.Printf("Mode       : %s\n", modeLabel())
	fmt.Println("Siap. Tempelkan kartu NFC pada reader... (Ctrl+C untuk keluar)")
	fmt.Println()

	if !*flagNoBrowser && !*flagList {
		openKiosk(*flagURL)
	}

	states := []scard.ReaderState{{Reader: name, CurrentState: scard.StateUnaware}}
	for {
		if err := ctx.GetStatusChange(states, 500*time.Millisecond); err != nil {
			if errors.Is(err, scard.ErrTimeout) {
				continue
			}
			if errors.Is(err, scard.ErrUnknownReader) {
				log.Fatalf("reader terputus/dicabut - jalankan ulang bridge")
			}
			if errors.Is(err, scard.ErrNoService) || errors.Is(err, scard.ErrServiceStopped) {
				log.Fatalf("Smart Card service berhenti - cek services.msc")
			}
			log.Fatalf("GetStatusChange: %v", err)
		}
		wasPresent := states[0].CurrentState&scard.StatePresent != 0
		nowPresent := states[0].EventState&scard.StatePresent != 0
		atr := append([]byte(nil), states[0].Atr...)
		if !wasPresent && nowPresent {
			handleTap(ctx, name, atr)
		}
		states[0].CurrentState = states[0].EventState
	}
}

func pickReader(readers []string, substr string) string {
	want := strings.ToLower(substr)
	for _, r := range readers {
		if strings.Contains(strings.ToLower(r), want) {
			return r
		}
	}
	log.Printf("reader cocok untuk '%s' tidak ditemukan, pakai reader pertama", substr)
	return readers[0]
}

func modeLabel() string {
	if *flagDryRun {
		return "DRY-RUN (UID hanya ditampilkan, tidak diketik)"
	}
	return "kirim ketikan UID + Enter ke jendela aktif (fokus harus di browser kiosk)"
}

// handleTap dipanggil saat kartu BARU ditempel (empty -> present).
func handleTap(ctx *scard.Context, reader string, atr []byte) {
	if *flagDebug && len(atr) > 0 {
		log.Printf("ATR: % X", atr)
	}
	time.Sleep(80 * time.Millisecond) // beri jeda settle kartu

	card, err := ctx.Connect(reader, scard.ShareShared, scard.ProtocolT0|scard.ProtocolT1)
	if err != nil {
		log.Printf("connect kartu gagal: %v", err)
		return
	}
	defer card.Disconnect(scard.LeaveCard)

	uid := ""
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := card.Transmit([]byte{0xFF, 0xCA, 0x00, 0x00, 0x00})
		if err != nil {
			log.Printf("transmit gagal (percobaan %d/3): %v", attempt, err)
		} else if len(resp) >= 2 {
			sw := resp[len(resp)-2:]
			if sw[0] == 0x90 && sw[1] == 0x00 {
				uid = strings.ToUpper(hex.EncodeToString(resp[:len(resp)-2]))
				break
			}
			log.Printf("kartu menolak baca UID (SW=% X, percobaan %d/3)", sw, attempt)
		} else {
			log.Printf("respon kosong (percobaan %d/3)", attempt)
		}
		time.Sleep(120 * time.Millisecond)
	}
	if uid == "" {
		log.Printf("GAGAL membaca UID - kartu mungkin tidak kompatibel. Angkat lalu coba kartu lain.")
		return
	}
	if len(uid) < 8 {
		log.Printf("UID tidak wajar (terlalu pendek): %s - diabaikan", uid)
		return
	}

	log.Printf("UID: %s (%d byte)", uid, len(uid)/2)
	if !*flagNoBeep {
		beepOK()
	}
	if *flagDryRun {
		log.Printf("[dry-run] UID tidak diketik. Gunakan nilai ini saat ENROLL: %s", uid)
		return
	}
	if err := typeString(uid); err != nil {
		log.Printf("gagal mengetik: %v", err)
		return
	}
	if !*flagNoEnter {
		time.Sleep(time.Duration(*flagDelay) * time.Millisecond)
		if err := sendKey(0x0D); err != nil { // Enter
			log.Printf("gagal kirim Enter: %v", err)
			return
		}
	}
	log.Printf("terkirim ke jendela aktif.")
}
