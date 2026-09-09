# Verifikasi Regulasi — Salinan Resmi 358/M/KEP/2025

**Sumber:** `docs/Salinan Nomor 358-M-KEP-2025-2.pdf` (48 hal., salinan resmi — diterima 19 Aug 2026)
**Objek:** definisi 12 IKU + formula seed di `iku_db` ( hasil F0 + perubahan F6)
**Tanggal verifikasi:** 2026-08-19 · **Verifikator:** Em (atas persetujuan dokumen oleh Mas Rama)

> Dokumen resmi memuat **12 nomor IKU bagi PT** + IKU LLDIKTI. Ringkasan blueprint B-Wise (12 IKU) **satu-satu cocok** dengan dokumen — pemetaan valid. Namun ada **3 temuan formula/sifat** yang perlu dikoreksi via Compliance Engine (regulation change), bukan hardcode.

## Pemetaan & Hasil Verifikasi

| # | IKU (dokumen resmi) | Sifat resmi | Formula resmi (ringkas) | Seed saat ini | Status |
|---|---|---|---|---|---|
| 1 | Angka Efisiensi Edukasi PT | WAJIB | 2 langkah: AEE = lulus tepat waktu / terdaftar ×100%; **Pencapaian = AEE realisasi / AEE Ideal ×100%** (ideal per jenjang: D1 100%, D2 50%, D3 33%, D4/S1 25%, dst.) | `lulus / terdaftar * 100` | ⚠️ Sebagian — langkah-2 (÷ AEE ideal per jenjang) belum |
| 2 | % Lulusan bekerja/wirausaha/lanjut | WAJIB | **Σ nᵢkᵢ / t ×100%** — pembobotan jenis pekerjaan + responden minimum (galat 2,3%) | count sederhana / lulusan | ⚠️ Penyederhanaan (tanpa bobot k) |
| 3 | % Mhs kegiatan/prestasi luar prodi | WAJIB | **Σ nᵢkᵢ / t ×100%** — bobot MBKM per SKS (≤5: 0,4; 6–10: 0,6; >10: 1) & prestasi (internasional juara 1: 1; nasional: 0,6; provinsi: 0,4) | `mahasiswa_mbkm / mahasiswa_aktif * 100` | ⚠️ Penyederhanaan (tanpa bobot) |
| 4 | % Dosen rekognisi internasional | PILIHAN | dosen rekognisi (NUPTK) / total dosen ×100% | `dosen_rekognisi / dosen_total * 100` | ✅ Sesuai |
| 5 | % Luaran kerjasama & hilirisasi dgn industri | WAJIB | **luaran kerjasama / TOTAL KERJASAMA PT ×100%** (jumlah judul/karya, bukan jumlah dosen) | `luaran_kerjasama / dosen_total` (tanpa %) | ❌ **Denominator & satuan salah** |
| 6 | % Publikasi bereputasi intl (Scopus/WoS) | WAJIB bagi **PTN-BH**; PILIHAN bagi PTN lain & **PTS** | **Σ nᵢkᵢ / t ×100%** — t = total publikasi PT; bobot: Top 1,2 / Q1 1,0 / Q2 0,75 / Q3 0,5 / Q4 0,25 / prosiding 0,25; kolaborasi intl +0,25; publisher ekslusif MDPI/Frontiers/Hindawi tidak dihitung | v2 (aktif, simulasi F6): `(publikasi * 1.2) / dosen_total` | ❌ **Formula beda total** (per-dosen → proporsi terbobot); sifat utk Binawan (PTS) = **pilihan** |
| 7 | % Keterlibatan SDGs | WAJIB | program SDG (1,4,17 + 2 pilihan) / total program ×100% | `kegiatan_sdg / kegiatan_total * 100` | ✅ Sesuai |
| 8 | % SDM penyusun kebijakan | PILIHAN | SDM terlibat kebijakan / total SDM ×100% | `sdm_kebijakan / sdm_total * 100` | ✅ Sesuai |
| 9 | % Pendapatan non-UKT | WAJIB | pendapatan non-mahasiswa / total pendapatan ×100% | `pendapatan_non_ukt / pendapatan_total * 100` | ✅ Sesuai |
| 10 | Zona Integritas WBK/WBBM | PILIHAN (PTN) | Satuan: **unit kerja** (jumlah unit yang mengajukan via KemenPANRB) | tanpa formula | ✅ (non-matematis) |
| 11 | a. Opini audit LKPT · b. Predikat SAKIP · c. Laporan pelanggaran integritas akademik · d. Anti kekerasan/narkoba/korupsi | PILIHAN (c & d: a. semakin rendah semakin baik) | Satuan: opini / predikat / jumlah laporan / % kegiatan terlaksana÷direncanakan | tanpa formula | ✅ (kualitatif — catatan: resmi punya **4 komponen**) |
| 12 | Perencanaan strategis kesejahteraan dosen | WAJIB bagi PT | Satuan: **dokumen** (Renstra/RIP SDM; standar penghasilan per jabatan: AA ≥1,5×UMP, Lektor ≥3×, LK ≥4×, Prof ≥6×) | tanpa formula | ✅ (non-matematis; standar UMP dicatat utk checklist) |

## Temuan Utama (perlu aksi)

1. **IKU-5 salah denominator** — seharusnya dibagi *total kerjasama PT* (jumlah judul/karya), bukan jumlah dosen; satuan persen.
2. **IKU-6 salah struktur formula** — resmi: proporsi publikasi **terbobot kuartil** terhadap *total publikasi*; bukan publikasi per dosen. Selain itu sifat resmi bagi **PTS = IKU PILIHAN** (field `nature` di master perlu keputusan).
3. **IKU-1/2/3 penyederhanaan** — formula resmi memakai **pembobotan (Σ nᵢkᵢ)** dan IKU-1 memakai rasio terhadap *AEE ideal per jenjang*. Seed saat ini masih sah sebagai baseline, tapi target resmi LLDIKTI akan memakai formula berbobot.

## Tindakan (sesuai prinsip Regulation-First — semua via Compliance Engine, bukan hardcode)

- [x] Dokumen resmi disimpan `docs/` + `regulatory_versions.document_url` diisi + status **verified**
- [x] **Draft regulation change** "Koreksi IKU-5 & IKU-6 sesuai Salinan resmi" dibuat (status **draft** — menunggu review Mas Rama via UI Compliance → Impact Analyzer → Approve → Activate)
- [ ] Keputusan Mas Rama: `nature` IKU-6 (wajib→pilihan untuk PTS?) — tercatat di atas
- [ ] Upgrade formula berbobot IKU-1/2/3 — menyusul (perlu desain variabel per-jenjang/kuartil bersama tim)

## Catatan Teknis

- Duplikat IKU-6 (lahir dari bug seed idempotency setelah aktivasi regulasi F6) sudah dibersihkan + seed diperbaiki (cek by `iku_code` global).
- Bobot resmi IKU-6: Top-tier 1,2 · Q1 1,0 · Q2 0,75 · Q3 0,5 · Q4 0,25 · prosiding 0,25 · kolaborasi intl +0,25 (di atas bobot dasar).
- Publisher yang TIDAK dihitung: MDPI, Frontiers, Hindawi.
- IKU-2: responden minimum n = N/(1+N·d²), d = 2,3%.
