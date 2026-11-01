package migrations

// AllMigrations returns the ordered list of migrations to run.
// Add your migrations here in chronological order.
func AllMigrations() []MigrationDef {
	return []MigrationDef{
		// Example:
		// {
		// 	Name: "001_create_default_data",
		// 	Up:   createDefaultData,
		// },
	}
}

// === Example migration ===
// func createDefaultData(db *gorm.DB) error {
// 	// Seed default data here
// 	return nil
// }
