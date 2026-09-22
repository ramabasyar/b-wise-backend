package migrations

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Migration represents a single database migration
type Migration struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)"`
	Name      string    `gorm:"type:varchar(255);not null"`
	AppliedAt time.Time `gorm:"autoCreateTime"`
}

func (Migration) TableName() string { return "migrations" }

// MigrationFunc is a function that performs a migration
type MigrationFunc func(db *gorm.DB) error

// MigrationDef defines a migration to run
type MigrationDef struct {
	Name string
	Up   MigrationFunc
}

// Run executes all pending migrations in order
func Run(db *gorm.DB, migrations []MigrationDef) error {
	// Create migrations tracking table
	if err := db.AutoMigrate(&Migration{}); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	for _, m := range migrations {
		// Check if already applied
		var count int64
		db.Model(&Migration{}).Where("name = ?", m.Name).Count(&count)
		if count > 0 {
			continue
		}

		fmt.Printf("[migration] running: %s\n", m.Name)

		// Run migration
		if err := m.Up(db); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.Name, err)
		}

		// Record as applied
		db.Create(&Migration{Name: m.Name})
		fmt.Printf("[migration] completed: %s\n", m.Name)
	}

	return nil
}
