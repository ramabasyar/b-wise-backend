# Binawan Library

Service B-Wise untuk perpustakaan Universitas Binawan — tahap awal: **pencatatan & pelaporan kunjungan perpustakaan**.

## Status (15 Okt 2026)
- **F0 — scaffold**: service dari starter kit, entity GORM + manifest permission/menu kunjungan siap; endpoint bisnis menyusul (F1).
- Dokumen rencana: `dokumen/B-Wise/Library/2026-10-15-Rencana-Pengembangan-Library-Kunjungan.md`

## Scope MVP
- Check-in anggota via **tap kartu NFC**; fallback input NIM saat lupa kartu (teraudit, rate-limited).
- Tamu via **barcode/QR pass** opaque ber-expiry (token acak, bisa dicabut).
- Registry anggota & kartu dikelola **Library Admin**: registrasi satuan, enroll/revoke/replace kartu, impor CSV (preview + idempotent upsert per NIM).
- Dashboard, laporan, ekspor CSV, audit lengkap.

Di luar scope: katalog buku, peminjaman/sirkulasi, smart locker (dibahas terpisah).

## Menjalankan (dev)
```bash
cp .env.example .env   # DB lokal: library_db; SSO dev di :8070 (bukan 8080)
make run               # listening di :8088
```

Service self-register permissions & menu via `perm.manifest.yaml` saat boot (butuh SSO service token + PERMISSION_SERVICE_URL di config).

## Catatan teknis
- **Port 8088 dipilih manual**: CLI mengusulkan 8086, tetapi port itu dipakai solver sidecar (FastAPI, lokal & nakula) yang tidak terdeteksi pemindai port CLI.
- Skema via GORM AutoMigrate (pola starter kit): `internal/domain/entity/library.go`.
- NFC reader kiosk: prioritas USB HID/keyboard; Web NFC tidak dipakai (dukungan desktop terbatas). Spike kartu asli wajib sebelum beli massal.
- Kredensial kartu disimpan sebagai HMAC lookup; raw credential tidak masuk log.
