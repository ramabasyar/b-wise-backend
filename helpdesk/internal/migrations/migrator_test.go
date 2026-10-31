package migrations

import "testing"

func TestMigrationBeforeCreateGeneratesUniqueIDs(t *testing.T) {
	first := &Migration{}
	second := &Migration{}
	if err := first.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if err := second.BeforeCreate(nil); err != nil {
		t.Fatal(err)
	}
	if len(first.ID) != 32 || len(second.ID) != 32 {
		t.Fatalf("expected 32-char IDs, got %q and %q", first.ID, second.ID)
	}
	if first.ID == second.ID {
		t.Fatal("migration IDs must be unique")
	}
}
