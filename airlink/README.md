# Airlink

Portal pengelolaan jaringan Binawan — service B-Wise untuk WiFi WPA2-Enterprise (EAP-TLS), monitoring sesi, dan infrastruktur RADIUS.

## Status (14 Okt 2026)
- **Tahap A+B (pintu masuk + observability)** — backend staging port 8090, frontend staging tersedia
- Rilis awal **khusus IT**; self-service staf menyusul
- Endpoint terbitkan, daftar, dan unduh ulang sertifikat sudah tersedia dengan paket+password terenkripsi di DB Airlink; **revoke dari portal belum dibuka** sampai CoA UniFi dan accounting stop teruji

## Fitur (rilis pertama)
- `GET /api/airlink/health` — status kesehatan RADIUS (DB, sesi aktif, auth terakhir)
- `GET /api/airlink/sessions/active` — sesi WiFi aktif (filter by username)
- `GET /api/airlink/sessions/history` — riwayat sesi (filter by username, start, end)
- `GET /api/airlink/auth-logs` — log autentikasi radpostauth
- `GET /api/airlink/certs` — metadata sertifikat yang diterbitkan Airlink (IT)
- `POST /api/airlink/certs` — penerbitan cert oleh IT
- `GET /api/airlink/certs/:username/:device/package` — unduh ulang paket dan password (diaudit)

Sertifikat yang diterbitkan sebelum Airlink belum memiliki password di penyimpanan terenkripsi dan tidak dapat diunduh ulang. Jaga backup terenkripsi `.env` (terutama `AIRLINK_ENCRYPTION_KEY`); kehilangan kunci berarti paket lama tidak dapat dibuka. Rute revoke sengaja belum diaktifkan: CRL saja tidak memutus sesi WiFi yang sudah aktif.

## Menjalankan (dev)
```bash
cp .env.example .env  # sesuaikan; jangan commit kredensial
make deps
make run  # listening di :8087
```

Service self-register permissions & menu via perm.manifest.yaml saat boot.
Butuh: SSO service token + Permission Service URL di config.

## Infrastruktur
- FreeRADIUS 3.2.5 @ 10.123.120.25 (EAP-TLS + VLAN 7 staff, CA Binawan WiFi Root CA)
- DB radius @ PG arjuna 10.123.123.22 (radcheck/radacct/radpostauth)
- Engine cert: issue-wifi-cert.sh & revoke-wifi-cert.sh (max 5/user, legacy format utk Apple)
- AP UniFi subnet 172.16.88.0/24 + controller 103.153.42.2

## Permission Roles
| Role | Hak |
|---|---|
| Airlink Operator | Baca status/sesi/log (read-only) |
| Airlink Admin | + Issue/unduh/cabut sertifikat, kelola limit |
