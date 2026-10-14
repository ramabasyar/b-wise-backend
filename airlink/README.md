# Airlink

Portal pengelolaan jaringan Binawan — service B-Wise untuk WiFi WPA2-Enterprise (EAP-TLS), monitoring sesi, dan infrastruktur RADIUS.

## Status (14 Okt 2026)
- **Tahap A+B (pintu masuk + observability)** — backend staging port 8090, frontend staging tersedia
- Rilis awal **khusus IT**; self-service staf menyusul
- Endpoint terbitkan, daftar, unduh ulang dan revoke sertifikat tersedia; paket+password terenkripsi di DB Airlink. Revoke mencabut CRL, lalu mencoba CoA disconnect per sesi; AP UniFi belum terbukti mengirim Disconnect-ACK sehingga UI wajib memperlihatkan hasil tiap sesi secara terpisah.

## Fitur (rilis pertama)
- `GET /api/airlink/health` — status kesehatan RADIUS (DB, sesi aktif, auth terakhir)
- `GET /api/airlink/sessions/active` — sesi WiFi aktif (filter by username)
- `GET /api/airlink/sessions/history` — riwayat sesi (filter by username, start, end)
- `GET /api/airlink/auth-logs` — log autentikasi radpostauth
- `GET /api/airlink/certs` — metadata sertifikat yang diterbitkan Airlink (IT)
- `POST /api/airlink/certs` — penerbitan cert oleh IT
- `GET /api/airlink/certs/:username/:device/package` — unduh ulang paket dan password (diaudit)
- `GET /api/airlink/dns/records` — daftar record PowerDNS
- `POST/PUT/DELETE /api/airlink/dns/records` — kelola A/AAAA/CNAME/TXT oleh IT, audit di `dns_change_audit`, lalu sinkronkan dnsmasq
- `POST /api/airlink/dns/sync` — sinkronkan ulang exact-name forwarding
- `GET /api/airlink/activity/dns` dan `/activity/netflow` — event terkini, dengan mapping IP→username bila accounting RADIUS cocok
- `GET /api/airlink/activity/summary` — total dan top domain/IP tujuan untuk periode 1–90 hari

Sertifikat yang diterbitkan sebelum Airlink belum memiliki password di penyimpanan terenkripsi dan tidak dapat diunduh ulang. Jaga backup terenkripsi `.env` (terutama `AIRLINK_ENCRYPTION_KEY`); kehilangan kunci berarti paket lama tidak dapat dibuka. CRL saja tidak memutus sesi aktif; disconnect via CoA masih perlu pembuktian dengan AP nyata.

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
- DNS: dnsmasq `.25:53` → PowerDNS `.25:5300` untuk nama internal terdaftar; nama publik tetap ke upstream. NetFlow pmacct `.25:2055/udp`.
- Query DNS dan NetFlow hanya indikasi teknis; bukan bukti halaman web tertentu dikunjungi. Raw telemetry belum punya retensi otomatis.

## Permission Roles
| Role | Hak |
|---|---|
| Airlink Operator | Baca status/sesi/log, record DNS, dan aktivitas jaringan (read-only) |
| Airlink Admin | + Issue/unduh/cabut sertifikat, kelola limit, ubah record DNS |
