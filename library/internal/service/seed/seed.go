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
