package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"nudge/internal/dto"
)

func TestFileConfigStoreLoad_WithDatabases(t *testing.T) {
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

	config := []byte(`{
  "databases": [
    {
      "key": "tasks",
      "name": "My Tasks",
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
    }
  ],
  "poll_interval_seconds": 30,
  "max_results": 42,
  "launch_at_login": true,
  "tray_icon_path": "tray.png",
  "notion_version": "2022-06-28"
}`)
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
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
	if taskDB.Name != "My Tasks" {
		t.Fatalf("tasks name = %q, want My Tasks", taskDB.Name)
	}
}

func TestFileConfigStoreLoad_EmptyConfig(t *testing.T) {
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

	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// デフォルト値が使われる
	if cfg.PollIntervalSeconds != 60 {
		t.Fatalf("PollIntervalSeconds = %d, want 60", cfg.PollIntervalSeconds)
	}
	if cfg.MaxResults != 30 {
		t.Fatalf("MaxResults = %d, want 30", cfg.MaxResults)
	}
	// databases は空
	if len(cfg.Databases) != 0 {
		t.Fatalf("Databases len = %d, want 0", len(cfg.Databases))
	}
}

func TestFileConfigStoreLoad_NoFile(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	store := NewFileConfigStore("NudgeTest")

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.PollIntervalSeconds != 60 {
		t.Fatalf("PollIntervalSeconds = %d, want 60", cfg.PollIntervalSeconds)
	}
	if len(cfg.Databases) != 0 {
		t.Fatalf("Databases len = %d, want 0", len(cfg.Databases))
	}
}

func TestFileConfigStoreLoad_IgnoresUnknownKeys(t *testing.T) {
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

	// brain_database_id や unknown_key など不明なキーを含む config.json
	config := []byte(`{
  "brain_database_id": "old-brain-db",
  "unknown_key": "some-value",
  "databases": [
    {
      "key": "tasks",
      "name": "Task List",
      "kind": "task",
      "enabled": true,
      "database_id": "db-123",
      "data_source_id": "",
      "title_property_name": "Name",
      "status_property_name": "Status",
      "status_property_type": "status",
      "status_in_progress": "In Progress",
      "status_done": "Done",
      "status_paused": "Paused",
      "checkbox_property_name": ""
    }
  ],
  "poll_interval_seconds": 30,
  "max_results": 10
}`)
	if err := os.WriteFile(path, config, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v (unknown keys should not cause errors)", err)
	}

	if cfg.PollIntervalSeconds != 30 {
		t.Fatalf("PollIntervalSeconds = %d, want 30", cfg.PollIntervalSeconds)
	}
	if cfg.MaxResults != 10 {
		t.Fatalf("MaxResults = %d, want 10", cfg.MaxResults)
	}

	taskDB, ok := cfg.DatabaseByKey("tasks")
	if !ok {
		t.Fatalf("tasks database not found")
	}
	if taskDB.DatabaseID != "db-123" {
		t.Fatalf("tasks database_id = %q, want db-123", taskDB.DatabaseID)
	}
	if taskDB.Name != "Task List" {
		t.Fatalf("tasks name = %q, want Task List", taskDB.Name)
	}
}

func TestFileConfigStoreSave(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))

	store := NewFileConfigStore("NudgeTest")

	cfg := dto.Config{
		Databases: []dto.DatabaseConfig{
			{
				Key:                "tasks",
				Name:               "Tasks",
				Kind:               dto.DatabaseKindTask,
				Enabled:            true,
				DatabaseID:         "db-1",
				StatusPropertyName: "Status",
				StatusPropertyType: "status",
				StatusInProgress:   "Doing",
				StatusDone:         "Done",
				StatusPaused:       "Paused",
			},
		},
		PollIntervalSeconds: 120,
		MaxResults:          50,
	}

	if err := store.Save(cfg); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	path, _ := store.Path()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved config: %v", err)
	}

	var saved dto.Config
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatalf("unmarshal saved config: %v", err)
	}

	if len(saved.Databases) != 1 || saved.Databases[0].Key != "tasks" {
		t.Fatalf("unexpected saved databases: %+v", saved.Databases)
	}
	if saved.PollIntervalSeconds != 120 {
		t.Fatalf("PollIntervalSeconds = %d, want 120", saved.PollIntervalSeconds)
	}
}
