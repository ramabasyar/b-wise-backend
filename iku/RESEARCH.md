# IKU_RESEARCH — Riset Sistem IKU Indonesia + eCakradipa UNDIP (input roadmap)

> **Status:** Draft riset 18 Aug 2026 — menunggu blueprint Mas Rama (v1 benchmark eCakradipa + v2) untuk finalisasi ROADMAP.md.
> **Sumber:** web (ecakradipa.apps.undip.ac.id, tp.undip.ac.id, SEVIMA/Kemendikbud, Sireva UNS, SAKIP/e-Kinerja).

## 1. Lanskap Sistem Kinerja di Indonesia

| Sistem | Scope | Pola kunci |
|---|---|---|
| **IKU Kemendikbudristek** (8 IKU PT) | Nasional — perguruan tinggi | 8 indikator tetap (lulusan kerja layak, MBKM, dosen praktisi, praktisi mengajar, kerja sama internasional, studi jangka pendek, prodi internasional, profil restrukturisasi) → dasar insentif pendanaan; pelaporan berkala berbasis capaian % target |
| **SAKIP** | Pemerintah (termasuk PTN) | Perjanjian kinerja (PK) → LKj triwulanan → review/verifikasi berjenjang → rating (A/B/C) — kaskade Renstra→Renop→program→kegiatan |
| **e-Kinerja BKN / IKI / SKP** | Individu pegawai | Kinerja individu tahunan (kuantitas/kualitas/waktu/biaya), triangulasi atasan — di kampus: dosen & tendik |
| **eCakradipa UNDIP** | Institusi (unit kerja) | **Benchmark utama** — detail di §2 |
| **Sireva UNS** | Institusi | Referensi IKU ter-kode (P-006 dst), capaian per unit per triwulan — pola mirip eCakradipa |

**Pola umum semua sistem** (harus ada di B-Wise Performance Center):
1. **Kaskade**: visi/Renstra → tujuan → sasaran/IKU institusi → turunan unit kerja → (opsional) individu
2. **Perjanjian Kinerja (PK)** dokumen tahunan per unit — target per indikator
3. **Periode pengukuran triwulanan** dengan timeline buka-tutup isian
4. **Capaian vs target** → % ketercapaian → status (deviasi/analisis)
5. **Verifikasi berjenjang** (operator → pimpinan unit → reviewer pusat/BPP) + status dokumen
6. **Bukti/dokumen pendukung** per capaian
7. **Dashboard rekap** per kluster/fakultas/tahun/triwulan + export laporan
8. **3 jenis indikator paralel** (di UNDIP): Kinerja UNDIP, IKU PTN BH, WCU

## 2. eCakradipa UNDIP — temuan detail

**Tagline:** "Rencanakan. Ukur. Evaluasi." — dari perjanjian kinerja hingga review capaian dalam satu aplikasi.

**Alur tahunan:**
1. Perjanjian Kinerja antara **pimpinan unit kerja ↔ Rektor** (dokumen tahunan, bisa di-export Word)
2. Pengukuran capaian **triwulanan** (IKU + Output) dengan jadwal resmi (mis. TW I: 13–24 April)
3. Input capaian oleh operator → **review & verifikasi berjenjang** oleh PIC IKU Kantor Pusat & BPP
4. Dashboard umum capaian per kluster (rerata %), filter tahun+triwulan, tampilan validasi "Reviewer + BPP"

**3 peran utama:**
- **Pimpinan unit kerja** — menandatangani PK, melihat capaian unit, analisis
- **Operator IKU** — input capaian indikator + bukti
- **Operator Output** — input capaian output kegiatan
- (+Reviewer/PIC pusat & BPP sebagai verifier berjenjang)

**Struktur indikator:** 3 set paralel — Indikator Kinerja UNDIP, IKU PTN BH, WCU — masing-masing dengan definisi & panduan terdokumentasi di dalam sistem.

**Implikasi desain untuk B-Wise (dari eCakradipa):**
- Unit kerja = departemen/branch dari **human-capital** (sinkron lintas service!)
- Peran = permission roles di **permission-service** (bukan tabel sendiri): `iku.operator`, `iku.head`, `iku.reviewer`
- Periode triwulan = tabel periode dengan status buka/tutup
- PK per unit per tahun = dokumen target set
- Capaian + bukti + verifikasi = inti transaksional
- Dashboard kluster & export laporan = fase lanjutan

## 3. Pertanyaan desain yang menunggu blueprint Mas Rama

1. Scope indikator: institusi-only atau turun ke individu (IKI/SKP pegawai)?
2. Jenis set indikator: apakah adopt 8 IKU Kemendikbud + set internal Binawan?
3. Verifikasi: berjenjang seperti eCakradipa (operator→pimpinan→pusat) atau lebih sederhana dulu?
4. Unit kerja: mapping ke branch (fakultas) atau department dari HC service?
5. Periode: triwulanan fixed atau fleksibel?
6. Blueprint v1 vs v2: apa saja delta yang harus dipakai (v2 = versi sempurna)?
