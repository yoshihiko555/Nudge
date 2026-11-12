package store

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"nudge/internal/dto"
)

func TestFileConfigStore_V2FieldsRoundTrip(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	s := NewFileConfigStore("NudgeTestV2")

	want := dto.DatabaseConfig{
		Key:                  "tasks",
		Kind:                 dto.DatabaseKindTask,
		DueDatePropertyName:  "期限日",
		PriorityPropertyName: "優先度",
		PriorityOrder:        []string{"高", "中", "低"},
		GoalPropertyName:     "",
	}
	if err := s.Save(dto.Config{Databases: []dto.DatabaseConfig{want}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := loaded.Databases[0]
	if got.DueDatePropertyName != want.DueDatePropertyName ||
		got.PriorityPropertyName != want.PriorityPropertyName ||
		!reflect.DeepEqual(got.PriorityOrder, want.PriorityOrder) {
		t.Fatalf("round trip mismatch: got %+v", got)
	}
}

func TestFileConfigStore_V1ConfigLoadsWithEmptyV2Fields(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	s := NewFileConfigStore("NudgeTestV1")

	path, err := s.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	v1 := `{
  "databases": [
    {"key": "tasks", "kind": "task", "enabled": true, "database_id": "db-1",
     "status_property_name": "Status", "status_property_type": "status",
     "status_in_progress": "Doing", "status_done": "Done", "status_paused": "Paused"}
  ],
  "poll_interval_seconds": 60,
  "max_results": 30
}`
	if err := os.WriteFile(path, []byte(v1), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	db := loaded.Normalize().Databases[0]
	if db.DueDatePropertyName != "" || db.PriorityPropertyName != "" || db.PriorityOrder != nil || db.GoalPropertyName != "" {
		t.Fatalf("v1 config must load with empty v2 fields: %+v", db)
	}
	if db.UsesDueDateQuery() {
		t.Fatalf("v1 config must keep the in-progress-only query")
	}
}
