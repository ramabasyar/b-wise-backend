package entity

import "time"

// LeaveType — master jenis cuti (CT/CB/CM/CS/CP/CTP...), dikelola admin HC.
type LeaveType struct {
	ID            string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Code          string    `json:"code" gorm:"type:varchar(30);uniqueIndex;not null"`
	Name          string    `json:"name" gorm:"type:varchar(100);not null"`
	Description   string    `json:"description,omitempty" gorm:"type:text"`
	DefaultDays   int       `json:"default_days" gorm:"not null;default:0"` // jatah/tahun — fallback terakhir
	CarryOverMax  float64   `json:"carry_over_max" gorm:"not null;default:0"` // maks sisa dibawa (0 = tidak boleh)
	AllowHalfDay  bool      `json:"allow_half_day" gorm:"not null;default:false"`
	IsPaid        bool      `json:"is_paid" gorm:"default:true"`
	Active        bool      `json:"active" gorm:"default:true"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// LeaveBalance — saldo cuti per (pegawai, jenis, tahun).
// Entitled bersumber dari kebijakan (LeavePolicy) → fallback LeaveType.DefaultDays.
type LeaveBalance struct {
	ID             string     `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	EmployeeID     string     `json:"employee_id" gorm:"type:varchar(36);uniqueIndex:uq_leave_balance,priority:1;not null"`
	LeaveTypeID    string     `json:"leave_type_id" gorm:"type:varchar(36);uniqueIndex:uq_leave_balance,priority:2;not null"`
	Year           int        `json:"year" gorm:"uniqueIndex:uq_leave_balance,priority:3;not null"`
	Entitled       float64    `json:"entitled" gorm:"not null;default:0"`
	CarriedOver    float64    `json:"carried_over" gorm:"not null;default:0"` // sisa tahun lalu (hangus setelah CarryExpiresAt)
	CarryExpiresAt *time.Time `json:"carry_expires_at,omitempty" gorm:"type:date"`
	Used           float64    `json:"used" gorm:"not null;default:0"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	LeaveType *LeaveType `json:"leave_type,omitempty" gorm:"foreignKey:LeaveTypeID"`
}

// LeaveRequest — pengajuan cuti + keputusan. Approval 1 LEVEL: atasan langsung
// (Employee.ReportsTo, snapshot ke ApproverID saat diajukan).
// Status: submitted → approved | rejected | cancelled (cancelled oleh pemilik selama submitted).
type LeaveRequest struct {
	ID           string     `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Number       string     `json:"number" gorm:"type:varchar(50);uniqueIndex;not null"` // LR-YYYYMM-####
	EmployeeID   string     `json:"employee_id" gorm:"type:varchar(36);index;not null"`
	LeaveTypeID  string     `json:"leave_type_id" gorm:"type:varchar(36);index;not null"`
	StartDate    time.Time  `json:"start_date" gorm:"type:date;not null"`
	EndDate      time.Time  `json:"end_date" gorm:"type:date;not null"`
	Days         float64    `json:"days" gorm:"not null"` // inklusif, dihitung server (0.5 = setengah hari)
	HalfDay      bool       `json:"half_day" gorm:"not null;default:false"`
	Reason       string     `json:"reason,omitempty" gorm:"type:text"`
	Status       string     `json:"status" gorm:"type:varchar(20);index;not null;default:'submitted'"`
	ApproverID   string     `json:"approver_id" gorm:"type:varchar(36);index"`
	DecisionNote string     `json:"decision_note,omitempty" gorm:"type:text"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
	SubmittedAt  time.Time  `json:"submitted_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	Employee  *Employee  `json:"employee,omitempty" gorm:"foreignKey:EmployeeID"`
	LeaveType *LeaveType `json:"leave_type,omitempty" gorm:"foreignKey:LeaveTypeID"`
	Approver  *Employee  `json:"approver,omitempty" gorm:"foreignKey:ApproverID"`
}

// LeavePolicy — kebijakan jatah cuti per kombinasi jenis kepegawaian × jenis cuti.
// EmploymentTypeID NULL = kebijakan default; fallback terakhir = field di LeaveType.
type LeavePolicy struct {
	ID                string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Name              string    `json:"name" gorm:"type:varchar(100);not null"`
	EmploymentTypeID  *string   `json:"employment_type_id,omitempty" gorm:"type:varchar(36);uniqueIndex:uq_leave_policy,priority:1"`
	LeaveTypeID       string    `json:"leave_type_id" gorm:"type:varchar(36);uniqueIndex:uq_leave_policy,priority:2;not null"`
	AnnualDays        float64   `json:"annual_days" gorm:"not null;default:0"`
	CarryOverMax      float64   `json:"carry_over_max" gorm:"not null;default:0"`
	CarryExpiryMonths int       `json:"carry_expiry_months" gorm:"not null;default:3"`
	AllowHalfDay      bool      `json:"allow_half_day" gorm:"not null;default:false"`
	Active            bool      `json:"active" gorm:"default:true"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	LeaveType *LeaveType `json:"leave_type,omitempty" gorm:"foreignKey:LeaveTypeID"`
}

// LeaveBlockDate — tanggal pengajuan cuti DILARANG (mis. masa ujian, hari penting kampus).
type LeaveBlockDate struct {
	ID        string    `json:"id" gorm:"type:varchar(36);primaryKey;default:gen_random_uuid()"`
	Date      time.Time `json:"date" gorm:"type:date;uniqueIndex;not null"`
	Reason    string    `json:"reason" gorm:"type:varchar(200);not null"`
	Active    bool      `json:"active" gorm:"default:true"`
	CreatedBy string    `json:"created_by,omitempty" gorm:"type:varchar(36)"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
