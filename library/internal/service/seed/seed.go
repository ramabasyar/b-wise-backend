package seed

import (
	"gorm.io/gorm"

	"github.com/rama/b-wise/library/internal/domain/entity"
)

// Defaults mengisi lokasi utama dan daftar tujuan kunjungan awal (idempotent).
func Defaults(db *gorm.DB, log func(string, ...interface{})) {
	var loc entity.LibraryLocation
	if err := db.Where(entity.LibraryLocation{Code: "main"}).
		Attrs(entity.LibraryLocation{Name: "Perpustakaan Utama", Timezone: "Asia/Jakarta", IsActive: true}).
		FirstOrCreate(&loc).Error; err != nil {
		log("[seed] location gagal: %v", err)
	} else {
		log("[seed] location 'main' siap (id=%s)", loc.ID)
	}

	purposes := []entity.LibraryVisitPurpose{
		{Code: "general", Name: "Umum", SortOrder: 1, IsDefault: true},
		{Code: "read_study", Name: "Membaca / Belajar", SortOrder: 2},
		{Code: "borrow_return", Name: "Pinjam / Kembali Buku", SortOrder: 3},
		{Code: "computer_internet", Name: "Komputer / Internet", SortOrder: 4},
		{Code: "assignment", Name: "Mengerjakan Tugas", SortOrder: 5},
		{Code: "event", Name: "Kegiatan / Event", SortOrder: 6},
		{Code: "other", Name: "Lainnya", SortOrder: 7},
	}
	for _, p := range purposes {
		var out entity.LibraryVisitPurpose
		if err := db.Where(entity.LibraryVisitPurpose{Code: p.Code}).
			Attrs(entity.LibraryVisitPurpose{Name: p.Name, SortOrder: p.SortOrder, IsDefault: p.IsDefault, IsActive: true}).
			FirstOrCreate(&out).Error; err != nil {
			log("[seed] purpose %s gagal: %v", p.Code, err)
		}
	}
	log("[seed] tujuan kunjungan siap (7)")
}

// FacultiesAndPrograms mengisi referensi fakultas & prodi resmi (SIAKAD 19 Okt 2026, idempotent).
// Code prodi = 2 digit pertama NIM; suffix P untuk prodi profesi yang kode NIM-nya sama dengan S1.
func FacultiesAndPrograms(db *gorm.DB, log func(string, ...interface{})) {
	faculties := []entity.LibraryFaculty{
		{Code: "FKUB01", Name: "Fakultas Fisioterapi", ShortName: "FFT", SortOrder: 1},
		{Code: "FKUB02", Name: "Fakultas Ilmu Sosial dan Humaniora", ShortName: "FISH", SortOrder: 2},
		{Code: "FKUB03", Name: "Fakultas Kesehatan Masyarakat", ShortName: "FKM", SortOrder: 3},
		{Code: "FKUB04", Name: "Fakultas Keperawatan dan Kebidanan", ShortName: "FKK", SortOrder: 4},
		{Code: "FKUB09", Name: "Fakultas Bisnis dan Ilmu Sosial", ShortName: "FBIS", SortOrder: 5},
		{Code: "FKUB10", Name: "Fakultas Ilmu Kesehatan dan Teknologi", ShortName: "FIKT", SortOrder: 6},
	}
	for _, f := range faculties {
		var out entity.LibraryFaculty
		if err := db.Where(entity.LibraryFaculty{Code: f.Code}).
			Attrs(entity.LibraryFaculty{Name: f.Name, ShortName: f.ShortName, SortOrder: f.SortOrder, IsActive: true}).
			FirstOrCreate(&out).Error; err != nil {
			log("[seed] faculty %s gagal: %v", f.Code, err)
		}
	}
	programs := []entity.LibraryStudyProgram{
		{FacultyCode: "FKUB04", Code: "01", Name: "S1 - Keperawatan", SortOrder: 1},
		{FacultyCode: "FKUB04", Code: "01P", Name: "Prof - Profesi Ners", SortOrder: 2},
		{FacultyCode: "FKUB10", Code: "02", Name: "D4 - Fisioterapi", SortOrder: 3},
		{FacultyCode: "FKUB10", Code: "03", Name: "D4 - Keselamatan dan Kesehatan Kerja", SortOrder: 4},
		{FacultyCode: "FKUB10", Code: "04", Name: "S1 - Gizi", SortOrder: 5},
		{FacultyCode: "FKUB04", Code: "05", Name: "D3 - Kebidanan", SortOrder: 6, IsActive: false}, // SIAKAD: tidak aktif
		{FacultyCode: "FKUB10", Code: "06", Name: "D4 - Teknologi Laboratorium Medis", SortOrder: 7},
		{FacultyCode: "FKUB10", Code: "07", Name: "S1 - Farmasi", SortOrder: 8},
		{FacultyCode: "FKUB09", Code: "08", Name: "S1 - Kesejahteraan Sosial", SortOrder: 9},
		{FacultyCode: "FKUB09", Code: "09", Name: "S1 - Manajemen", SortOrder: 10},
		{FacultyCode: "FKUB09", Code: "10", Name: "S1 - Psikologi", SortOrder: 11},
		{FacultyCode: "FKUB10", Code: "11", Name: "S1 - Teknik Fisika", SortOrder: 12, IsActive: false}, // ditutup: tanpa mahasiswa (Rama 19 Okt 2026)
		{FacultyCode: "FKUB10", Code: "12", Name: "S1 - Teknik Lingkungan", SortOrder: 13, IsActive: false}, // ditutup: tanpa mahasiswa (Rama 19 Okt 2026)
		{FacultyCode: "FKUB02", Code: "13", Name: "S1 - Bahasa Inggris", SortOrder: 14, IsActive: false}, // SIAKAD: tidak aktif
		{FacultyCode: "FKUB10", Code: "14", Name: "S1 - Bioteknologi", SortOrder: 15, IsActive: false}, // ditutup: tanpa mahasiswa (Rama 19 Okt 2026)
		{FacultyCode: "FKUB10", Code: "15", Name: "S1 - Ilmu Aktuaria", SortOrder: 16, IsActive: false}, // ditutup: tanpa mahasiswa (Rama 19 Okt 2026)
		{FacultyCode: "FKUB09", Code: "16", Name: "S1 - Bisnis Digital", SortOrder: 17},
		{FacultyCode: "FKUB04", Code: "17", Name: "S1 - Kebidanan", SortOrder: 18},
		{FacultyCode: "FKUB04", Code: "17P", Name: "Prof - Pendidikan Profesi Bidan", SortOrder: 19},
		{FacultyCode: "FKUB10", Code: "18", Name: "S2 - Kesehatan Masyarakat", SortOrder: 20},
	}
	for _, p := range programs {
		var out entity.LibraryStudyProgram
		if err := db.Where(entity.LibraryStudyProgram{Code: p.Code}).
			Attrs(entity.LibraryStudyProgram{FacultyCode: p.FacultyCode, Name: p.Name, SortOrder: p.SortOrder, IsActive: p.IsActive}).
			FirstOrCreate(&out).Error; err != nil {
			log("[seed] program %s gagal: %v", p.Code, err)
		}
	}
	log("[seed] referensi fakultas (6) & prodi (%d, %d nonaktif) siap", len(programs), 6)
}
