# PILOT — B-Wise Performance Center (IKU)

Status: **siap pilot** (post-F7). Target: 2–3 unit kerja nyata, 1 periode penuh (TW atau tahunan).

## Unit pilot (data dev sudah tersedia)

| Unit | Role HC | Fokus uji |
|---|---|---|
| FIKK (Fakultas Ilmu Keperawatan) | branch | Input capaian manual + evidence + workflow submit→review |
| FIKST (Fakultas Ilmu Kesehatan ST) | branch | Import Excel/CSV terkelola + threshold warning |
| FBIS (Fakultas Bisnis & Informatika) | branch | Action plan (saran→buat→progress→done) + laporan export |

Unit "institution" (BPM/WR) = reviewer/publisher + dashboard rollup + compliance monitor.

## Checklist pra-pilot (operator BPM)

- [ ] Role & permission per akun pilot via permission-service (`achievements.write` utk operator unit, `achievements.workflow` utk reviewer, `compliance.read` utk pimpinan)
- [ ] Akun SSO operator tiap unit dibuat & di-assign role IKU sesuai RACI blueprint
- [ ] Periode TW1 2026 dibuka (periods.write), deadline input ditetapkan
- [ ] Target per unit ditetapkan (targets.write) — PK-lite export tersedia
- [ ] Formula 12 IKU diverifikasi vs JDIH 358/2025 (Compliance → regulasi verified + document_url)
- [ ] SMTP/WA webhook env diisi bila eskalasi email/WA ingin aktif (default: in-app + log)

## Skenario UAT per operator unit (1 siklus penuh)

1. Login portal → pilih service IKU → sidebar hanya menu sesuai permission
2. Input capaian manual (form auto dari variabel formula) → submit
3. Upload evidence (PDF/PNG ≤10MB) → lihat muncul di detail
4. Import Excel (template CSV/Excel) → preview → apply → capaian draft source=import
5. Terima rejection (uji notifikasi in-app + catatan reviewer) → revisi → submit ulang
6. Reviewer: approve → publish → lihat dashboard unit ter-update
7. Pimpinan: dashboard rollup institusi + export Excel/PDF + compliance monitor

## Metrik sukses pilot

- ≥90% capaian TW masuk sebelum deadline
- Workflow rejection→resubmit < 3 hari
- 0 data ganda (guard import re-apply bekerja)
- Feedback UX dicatat → iterasi (backlog F8)

## Go-live (rollout seluruh unit)

1. Backup penuh (`deploy/backup-iku.sh`) + restore test di environment staging
2. Pelatihan operator (materi: langkah UAT di atas)
3. Buka periode berikutnya untuk semua fakultas/prodi
4. Monitoring: `/api/metrics` + log terstruktur; review compliance mingguan
