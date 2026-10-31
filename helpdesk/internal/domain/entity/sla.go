package entity

// SLAPolicy — target respons & penyelesaian per prioritas (SL-01); pause_statuses =
// daftar status yang menjeda jam SLA (SL-03).
type SLAPolicy struct {
	Base
	Name          string `json:"name" gorm:"type:varchar(100);not null"`
	Targets       string `json:"targets" gorm:"type:jsonb"`
	PauseStatuses string `json:"pause_statuses,omitempty" gorm:"type:jsonb"`
	WarnPct       int    `json:"warn_pct" gorm:"default:75"`
}

func (SLAPolicy) TableName() string { return "sla_policies" }

// BusinessCalendar — jam layanan mingguan tim; libur dari Kalender Akademik Scheduling (SL-02).
type BusinessCalendar struct {
	Base
	Name          string `json:"name" gorm:"type:varchar(100);not null"`
	WeeklyHours   string `json:"weekly_hours" gorm:"type:jsonb"`
	Timezone      string `json:"timezone" gorm:"type:varchar(40);default:'Asia/Jakarta'"`
	HolidaySource string `json:"holiday_source" gorm:"type:varchar(20);default:'scheduling'"`
	IsActive      bool   `json:"is_active" gorm:"default:true;index"`
}

func (BusinessCalendar) TableName() string { return "business_calendars" }
