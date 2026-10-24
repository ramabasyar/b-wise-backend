package main

import (
	"github.com/rama/b-wise/human-capital/internal/adapter/logger"
	"github.com/rama/b-wise/human-capital/internal/domain/entity"
	"gorm.io/gorm"
)

// seedLeaveTypes — seed awal master jenis cuti (Fase 2).
// IDEMPOTEN: hanya insert kalau tabel kosong; setelah itu milik admin HC (edit via UI Pengaturan Cuti).
func seedLeaveTypes(db *gorm.DB, lg *logger.Logger) {
	if cnt(&entity.LeaveType{}, db) == 0 {
		rows := []entity.LeaveType{
			{Code: "CT", Name: "Cuti Tahunan", DefaultDays: 12, IsPaid: true, Active: true, Description: "Jatah 12 hari per tahun"},
			{Code: "CB", Name: "Cuti Besar", DefaultDays: 90, IsPaid: true, Active: true, Description: "Masa kerja > 5 tahun"},
			{Code: "CM", Name: "Cuti Melahirkan", DefaultDays: 90, IsPaid: true, Active: true, Description: "Sesuai UU Ketenagakerjaan"},
			{Code: "CS", Name: "Cuti Sakit", DefaultDays: 14, IsPaid: true, Active: true, Description: "Wajib surat keterangan dokter"},
			{Code: "CP", Name: "Cuti Alasan Penting", DefaultDays: 30, IsPaid: false, Active: true, Description: "Keperluan penting di luar cuti tahunan"},
			{Code: "CTP", Name: "Cuti Tidak Dibayar", DefaultDays: 30, IsPaid: false, Active: true, Description: "Persetujuan atasan diperlukan"},
		}
		db.Create(&rows)
		lg.Info("[seed] leave_types: 6 baris")
	}
}
