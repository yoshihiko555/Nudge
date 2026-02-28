package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"nudge/internal/dto"
)

const (
	legacyBrainDatabaseIDKey = "brain_database_id"
	legacyBrainTemplateIDKey = "brain_template_page_id"
)

type ConfigStore interface {
	Load() (dto.Config, error)
	Save(cfg dto.Config) error
	Path() (string, error)
}

type FileConfigStore struct {
	AppName string
}

func NewFileConfigStore(appName string) *FileConfigStore {
	return &FileConfigStore{AppName: appName}
}

func (s *FileConfigStore) Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, s.AppName, "config.json"), nil
}

func (s *FileConfigStore) Load() (dto.Config, error) {
	cfg := dto.DefaultConfig()
	path, err := s.Path()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	var raw struct {
		Databases           []dto.DatabaseConfig `json:"databases"`
		DatabaseID          string               `json:"database_id"`
		DataSourceID        string               `json:"data_source_id"`
		TitlePropertyName   string               `json:"title_property_name"`
		StatusPropertyName  string               `json:"status_property_name"`
		StatusPropertyType  string               `json:"status_property_type"`
		StatusInProgress    string               `json:"status_in_progress"`
		StatusDone          string               `json:"status_done"`
		StatusPaused        string               `json:"status_paused"`
		PollIntervalSeconds int                  `json:"poll_interval_seconds"`
		MaxResults          int                  `json:"max_results"`
		LaunchAtLogin       bool                 `json:"launch_at_login"`
		TrayIconPath        string               `json:"tray_icon_path"`
		NotionVersion       string               `json:"notion_version"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	if len(raw.Databases) > 0 {
		cfg.Databases = raw.Databases
	} else if raw.DatabaseID != "" || raw.DataSourceID != "" || raw.TitlePropertyName != "" {
		cfg.Databases = []dto.DatabaseConfig{
			{
				Key:                "tasks",
				Name:               "タスク",
				Kind:               dto.DatabaseKindTask,
				Enabled:            true,
				DatabaseID:         raw.DatabaseID,
				DataSourceID:       raw.DataSourceID,
				TitlePropertyName:  raw.TitlePropertyName,
				StatusPropertyName: raw.StatusPropertyName,
				StatusPropertyType: raw.StatusPropertyType,
				StatusInProgress:   raw.StatusInProgress,
				StatusDone:         raw.StatusDone,
				StatusPaused:       raw.StatusPaused,
			},
			{
				Key:     "habits",
				Name:    "習慣",
				Kind:    dto.DatabaseKindHabit,
				Enabled: true,
			},
		}
	}

	if raw.PollIntervalSeconds > 0 {
		cfg.PollIntervalSeconds = raw.PollIntervalSeconds
	}
	if raw.MaxResults > 0 {
		cfg.MaxResults = raw.MaxResults
	}
	cfg.LaunchAtLogin = raw.LaunchAtLogin
	cfg.TrayIconPath = raw.TrayIconPath
	if raw.NotionVersion != "" {
		cfg.NotionVersion = raw.NotionVersion
	}

	if err := s.migrateLegacyBrainKeys(path, b); err != nil {
		return cfg, err
	}
	return cfg.Normalize(), nil
}

func (s *FileConfigStore) Save(cfg dto.Config) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func (s *FileConfigStore) migrateLegacyBrainKeys(path string, rawJSON []byte) error {
	migratedJSON, updated, err := stripLegacyBrainKeys(rawJSON)
	if err != nil {
		return fmt.Errorf("parse config for migration: %w", err)
	}
	if !updated {
		return nil
	}
	if err := os.WriteFile(path, migratedJSON, 0o600); err != nil {
		return fmt.Errorf("write migrated config: %w", err)
	}
	return nil
}

func stripLegacyBrainKeys(rawJSON []byte) ([]byte, bool, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rawJSON, &raw); err != nil {
		return nil, false, err
	}

	_, hasDatabaseID := raw[legacyBrainDatabaseIDKey]
	_, hasTemplateID := raw[legacyBrainTemplateIDKey]
	if !hasDatabaseID && !hasTemplateID {
		return nil, false, nil
	}

	delete(raw, legacyBrainDatabaseIDKey)
	delete(raw, legacyBrainTemplateIDKey)
	migratedJSON, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return migratedJSON, true, nil
}
