# Library Visitor (library_visitor.exe)

Bridge PC/SC untuk kiosk perpustakaan Binawan Library: saat dijalankan otomatis membuka halaman kiosk (Edge fullscreen kiosk mode, fallback browser default) lalu membuat reader ACS ACR122U bekerja seperti USB scanner keyboard — UID kartu NFC di-inject sebagai ketikan (+ Enter) ke jendela aktif. Halaman kiosk \`/kiosk/library\` tidak perlu diubah sama sekali.

## Cara kerja
- Loop \`SCardGetStatusChange\` (timeout 500ms), edge-trigger pada
  transisi kartu EMPTY -> PRESENT (kartu menempel baru dikirim).
- Baca UID via APDU \`FF CA 00 00 00\` (ACS Get Data UID), 3x percobaan.
- Format UID: hex UPPERCASE tanpa pemisah — konsisten dengan
  \`credhash.Normalize\` di backend library (enroll & check-in sama-sama
  dinormalisasi sebelum HMAC lookup).
- Ketik via Win32 \`SendInput\` (virtual key 0-9 A-F), per karakter dengan
  jeda \`--delay\` (default 8ms), lalu Enter (\`--no-enter\` untuk menonaktifkan).
- Beep \`MessageBeep\` sebagai umpan balik (\`--no-beep\` untuk menonaktifkan).
- Auto-open browser saat start (\`--url\`, default staging kiosk; \`--no-browser\`
  menonaktifkan) via \`cmd /c start msedge --kiosk ...\`.
- Single-instance via named mutex \`Local\\bwise_library_visitor\`
  (instance kedua langsung keluar dengan pesan).

## Build
Lokali (darwin, stub inject): \`go build -o library_visitor .\`
Target kiosk Windows:

    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o library_visitor.exe .

Dependensi: github.com/ebfe/scard (pure syscall untuk Windows — zconst.go,
tanpa cgo). Module ini sengaja terpisah (nested module) agar tidak memengaruhi
build service library.

## Flag
    -reader string   substring nama reader (default "ACR122")
    -list            daftar reader lalu keluar
    -dry-run         tampilkan UID tanpa mengetik (mode uji/enroll)
    -delay int       jeda antar ketikan ms (default 8)
    -no-beep         tanpa beep
    -no-enter        tanpa Enter di akhir
    -debug           log ATR dll
    -url string      URL kiosk yang dibuka otomatis (default staging)
    -no-browser      jangan buka browser otomatis

## Catatan
- Fokus keyboard HARUS di jendela browser kiosk (halaman kiosk auto-fokus
  ke input tersembunyi; klik sekali di halaman bila ragu).
- Kartu yang menolak \`FF CA\` (mis. kartu bank) diabaikan dengan log.
- Enroll kartu bisa memanfaatkan mode normal: buka form "Daftarkan Kartu"
  di portal, klik field credential, lalu tap kartu — UID langsung terketik.
