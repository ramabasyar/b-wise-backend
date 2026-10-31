package migrations

import (
	"errors"

	"github.com/rama/b-wise/helpdesk/internal/domain/entity"
	"gorm.io/gorm"
)

// AllMigrations returns the ordered list of migrations to run.
// Add your migrations here in chronological order.
func AllMigrations() []MigrationDef {
	return []MigrationDef{
		{
			Name: "001_seed_default_sla",
			Up:   seedDefaultSLA,
		},
	}
}

func seedDefaultSLA(db *gorm.DB) error {
	calendar := entity.BusinessCalendar{}
	err := db.Where("name = ?", "Standar (Senin-Jumat 08:00-16:00)").First(&calendar).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		calendar = entity.BusinessCalendar{
			Name:          "Standar (Senin-Jumat 08:00-16:00)",
			WeeklyHours:   `{"mon":[{"start":"08:00","end":"16:00"}],"tue":[{"start":"08:00","end":"16:00"}],"wed":[{"start":"08:00","end":"16:00"}],"thu":[{"start":"08:00","end":"16:00"}],"fri":[{"start":"08:00","end":"16:00"}]}`,
			Timezone:      "Asia/Jakarta",
			HolidaySource: "manual",
			IsActive:      true,
		}
		if err := db.Create(&calendar).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	policy := entity.SLAPolicy{}
	err = db.Where("name = ?", "Standar").First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		policy = entity.SLAPolicy{
			Name:          "Standar",
			Targets:       `{"urgent":{"response_min":60,"resolution_min":480},"high":{"response_min":120,"resolution_min":1440},"normal":{"response_min":240,"resolution_min":2880},"low":{"response_min":480,"resolution_min":4800}}`,
			PauseStatuses: `["menunggu_pemohon","menunggu_approval"]`,
			WarnPct:       75,
		}
		if err := db.Create(&policy).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return db.Model(&entity.Team{}).
		Where("calendar_id IS NULL OR calendar_id = ?", "").
		Update("calendar_id", calendar.ID).Error
}
