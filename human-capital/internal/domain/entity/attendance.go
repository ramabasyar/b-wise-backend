package entity

import "time"

// AttendanceRecord — presensi harian per pegawai (1 baris per pegawai per tanggal).
// Sumber: check-in/out mandiri (source=app), input manual admin (source=manual),
// atau impor CSV dari mesin absensi (source=import).
type AttendanceRecord struct {
	ID         string     `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	EmployeeID string     `json:"employee_id" gorm:"type:varchar(36);uniqueIndex:uq_attendance,priority:1;not null;index"`
	Date       time.Time  `json:"date" gorm:"type:date;uniqueIndex:uq_attendance,priority:2;not null"`
	CheckIn    *time.Time `json:"check_in,omitempty" gorm:"type:timestamptz"`
	CheckOut   *time.Time `json:"check_out,omitempty" gorm:"type:timestamptz"`
	Status     string     `json:"status" gorm:"type:varchar(20);index;not null;default:'present'"`
	Source     string     `json:"source" gorm:"type:varchar(20);not null;default:'app'"`
	Notes      string     `json:"notes,omitempty" gorm:"type:varchar(255)"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

	Employee *Employee `json:"employee,omitempty" gorm:"foreignKey:EmployeeID"`
}
