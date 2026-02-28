package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nudge/internal/dto"
)

func TestFileConfigStoreLoad_MigratesLegacyBrainKeys(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	store := NewFileConfigStore("NudgeTest")
	path, err := store.Path()
	if err != nil {
		t.Fatalf("Path() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	legacyConfig := []byte(`{
  "databases": [
    {
      "key": "tasks",
      "name": "タスク",
      "kind": "task",
      "enabled": true,
      "database_id": "task-db",
      "data_source_id": "task-source",
      "title_property_name": "Name",
      "status_property_name": "Status",
      "status_property_type": "status",
      "status_in_progress": "In Progress",
      "status_done": "Done",
      "status_paused": "Paused",
      "checkbox_property_name": ""
    },
    {
      "key": "habits",
      "name": "習慣",
      "kind": "habit",
      "enabled": true,
      "database_id": "habit-db",
      "data_source_id": "habit-source",
      "title_property_name": "名前",
      "status_property_name": "",
      "status_property_type": "status",
      "status_in_progress": "",
      "status_done": "",
      "status_paused": "",
      "checkbox_property_name": "日,月,火,水,木,金,土"
    }
  ],
  "poll_interval_seconds": 30,
  "max_results": 42,
  "launch_at_login": true,
  "tray_icon_path": "tray.png",
  "notion_version": "2022-06-28",
  "brain_database_id": "legacy-brain-db",
  "brain_template_page_id": "legacy-brain-template",
  "custom_key": "kept"
}`)
	if err := os.WriteFile(path, legacyConfig, 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PollIntervalSeconds != 30 {
		t.Fatalf("PollIntervalSeconds = %d, want 30", cfg.PollIntervalSeconds)
	}
	if cfg.MaxResults != 42 {
		t.Fatalf("MaxResults = %d, want 42", cfg.MaxResults)
	}
	if !cfg.LaunchAtLogin {
		t.Fatalf("LaunchAtLogin = false, want true")
	}
	if cfg.TrayIconPath != "tray.png" {
		t.Fatalf("TrayIconPath = %q, want tray.png", cfg.TrayIconPath)
	}
	if cfg.NotionVersion != "2022-06-28" {
		t.Fatalf("NotionVersion = %q, want 2022-06-28", cfg.NotionVersion)
	}

	taskDB, ok := cfg.DatabaseByKey("tasks")
	if !ok {
		t.Fatalf("tasks database not found")
	}
	if taskDB.DataSourceID != "task-source" {
		t.Fatalf("tasks data_source_id = %q, want task-source", taskDB.DataSourceID)
	}

	habitDB, ok := cfg.DatabaseByKey("habits")
	if !ok {
		t.Fatalf("habits database not found")
	}
	if habitDB.DataSourceID != "habit-source" {
		t.Fatalf("habits data_source_id = %q, want habit-source", habitDB.DataSourceID)
	}

	migratedRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}

	var migratedMap map[string]json.RawMessage
	if err := json.Unmarshal(migratedRaw, &migratedMap); err != nil {
		t.Fatalf("unmarshal migrated config: %v", err)
	}

	if _, exists := migratedMap[legacyBrainDatabaseIDKey]; exists {
		t.Fatalf("migrated config still contains %q", legacyBrainDatabaseIDKey)
	}
	if _, exists := migratedMap[legacyBrainTemplateIDKey]; exists {
		t.Fatalf("migrated config still contains %q", legacyBrainTemplateIDKey)
	}
	if _, exists := migratedMap["custom_key"]; !exists {
		t.Fatalf("migrated config dropped custom_key")
	}

	var migratedStruct struct {
		Databases []dto.DatabaseConfig `json:"databases"`
	}
	if err := json.Unmarshal(migratedRaw, &migratedStruct); err != nil {
		t.Fatalf("unmarshal migrated databases: %v", err)
	}
	if len(migratedStruct.Databases) != 2 {
		t.Fatalf("migrated databases len = %d, want 2", len(migratedStruct.Databases))
	}
}

func TestStripLegacyBrainKeys_NoLegacyKeys(t *testing.T) {
	raw := []byte(`{"databases":[],"max_results":10}`)

	migrated, updated, err := stripLegacyBrainKeys(raw)
	if err != nil {
		t.Fatalf("stripLegacyBrainKeys() error = %v", err)
	}
	if updated {
		t.Fatalf("updated = true, want false")
	}
	if migrated != nil {
		t.Fatalf("migrated = %q, want nil", string(migrated))
	}
}
