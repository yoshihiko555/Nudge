package dto

import (
	"testing"
)

// TestDefaultConfig_EmptyDatabases verifies that DefaultConfig returns a Config
// with an empty Databases slice and the expected default values.
func TestDefaultConfig_EmptyDatabases(t *testing.T) {
	// Arrange / Act
	cfg := DefaultConfig()

	// Assert
	if len(cfg.Databases) != 0 {
		t.Errorf("expected empty Databases, got %d element(s)", len(cfg.Databases))
	}
	if cfg.PollIntervalSeconds != 60 {
		t.Errorf("expected PollIntervalSeconds=60, got %d", cfg.PollIntervalSeconds)
	}
	if cfg.MaxResults != 30 {
		t.Errorf("expected MaxResults=30, got %d", cfg.MaxResults)
	}
}

// TestNormalize_EmptyDatabases verifies that calling Normalize() on a Config with
// no databases leaves Databases empty (no default database is injected).
func TestNormalize_EmptyDatabases(t *testing.T) {
	// Arrange
	cfg := Config{}

	// Act
	result := cfg.Normalize()

	// Assert
	if len(result.Databases) != 0 {
		t.Errorf("expected empty Databases after Normalize(), got %d element(s)", len(result.Databases))
	}
}

// TestNormalize_ExistingDatabases verifies that Normalize() fills in missing Key
// and Kind fields when Databases are present.
func TestNormalize_ExistingDatabases(t *testing.T) {
	// Arrange
	cfg := Config{
		Databases: []DatabaseConfig{
			{Key: "", Kind: ""},
		},
	}

	// Act
	result := cfg.Normalize()

	// Assert
	if len(result.Databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(result.Databases))
	}
	db := result.Databases[0]
	if db.Key != "db-1" {
		t.Errorf("expected Key='db-1', got %q", db.Key)
	}
	if db.Kind != DatabaseKindTask {
		t.Errorf("expected Kind=%q, got %q", DatabaseKindTask, db.Kind)
	}
}

// TestNormalize_DuplicateKeys verifies that Normalize() resolves duplicate keys
// with incrementing counters instead of infinite looping.
func TestNormalize_DuplicateKeys(t *testing.T) {
	cfg := Config{
		Databases: []DatabaseConfig{
			{Key: "tasks", Kind: DatabaseKindTask},
			{Key: "tasks", Kind: DatabaseKindTask},
			{Key: "tasks", Kind: DatabaseKindTask},
		},
	}

	result := cfg.Normalize()

	if len(result.Databases) != 3 {
		t.Fatalf("expected 3 databases, got %d", len(result.Databases))
	}
	keys := map[string]bool{}
	for _, db := range result.Databases {
		if keys[db.Key] {
			t.Fatalf("duplicate key found: %q", db.Key)
		}
		keys[db.Key] = true
	}
	if result.Databases[0].Key != "tasks" {
		t.Errorf("expected first key='tasks', got %q", result.Databases[0].Key)
	}
	if result.Databases[1].Key != "tasks-2" {
		t.Errorf("expected second key='tasks-2', got %q", result.Databases[1].Key)
	}
	if result.Databases[2].Key != "tasks-3" {
		t.Errorf("expected third key='tasks-3', got %q", result.Databases[2].Key)
	}
}

// TestNormalize_HabitNoAutoFill verifies that Normalize() does NOT auto-fill
// TitlePropertyName or CheckboxPropertyName for a habit-kind database.
func TestNormalize_HabitNoAutoFill(t *testing.T) {
	// Arrange
	cfg := Config{
		Databases: []DatabaseConfig{
			{
				Key:                  "habit-db",
				Kind:                 DatabaseKindHabit,
				TitlePropertyName:    "",
				CheckboxPropertyName: "",
			},
		},
	}

	// Act
	result := cfg.Normalize()

	// Assert
	if len(result.Databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(result.Databases))
	}
	db := result.Databases[0]
	if db.TitlePropertyName != "" {
		t.Errorf("expected TitlePropertyName to remain empty, got %q", db.TitlePropertyName)
	}
	if db.CheckboxPropertyName != "" {
		t.Errorf("expected CheckboxPropertyName to remain empty, got %q", db.CheckboxPropertyName)
	}
}
