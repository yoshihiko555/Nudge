package dto

import (
	"fmt"
	"strings"
)

const (
	DatabaseKindTask  = "task"
	DatabaseKindHabit = "habit"
	DefaultHabitDays  = "日,月,火,水,木,金,土"
)

// DatabaseConfig はデータベースごとの設定。
type DatabaseConfig struct {
	Key                  string `json:"key"`
	Name                 string `json:"name"`
	Kind                 string `json:"kind"` // "task" or "habit"
	Enabled              bool   `json:"enabled"`
	DatabaseID           string `json:"database_id"`
	DataSourceID         string `json:"data_source_id"`
	TitlePropertyName    string `json:"title_property_name"`
	StatusPropertyName   string `json:"status_property_name"`
	StatusPropertyType   string `json:"status_property_type"` // "status" or "select"
	StatusInProgress     string `json:"status_in_progress"`
	StatusDone           string `json:"status_done"`
	StatusPaused         string `json:"status_paused"`
	CheckboxPropertyName string `json:"checkbox_property_name"`
}

// Config はローカル設定ファイルの内容。
type Config struct {
	Databases           []DatabaseConfig `json:"databases"`
	PollIntervalSeconds int              `json:"poll_interval_seconds"`
	MaxResults          int              `json:"max_results"`
	LaunchAtLogin       bool             `json:"launch_at_login"`
	TrayIconPath        string           `json:"tray_icon_path"`
	NotionVersion       string           `json:"notion_version"`
}

func DefaultConfig() Config {
	cfg := Config{
		PollIntervalSeconds: 60,
		MaxResults:          30,
	}
	cfg.Databases = defaultDatabases()
	return cfg
}

func (c Config) Normalize() Config {
	if len(c.Databases) == 0 {
		c.Databases = defaultDatabases()
	}
	c.Databases = normalizeDatabases(c.Databases)
	return c
}

func (c Config) DatabaseByKey(key string) (DatabaseConfig, bool) {
	for _, db := range c.Databases {
		if db.Key == key {
			return db, true
		}
	}
	return DatabaseConfig{}, false
}

func (c Config) FirstDatabaseByKind(kind string) (DatabaseConfig, bool) {
	for _, db := range c.Databases {
		if db.Kind == kind {
			return db, true
		}
	}
	return DatabaseConfig{}, false
}

func (d DatabaseConfig) ValidateForTaskQuery(notionVersion string, statusValue string) error {
	if d.DataSourceID == "" {
		return fmt.Errorf("data_source_id is required")
	}
	if d.TitlePropertyName == "" {
		return fmt.Errorf("title_property_name is required")
	}
	if d.StatusPropertyName == "" {
		return fmt.Errorf("status_property_name is required")
	}
	if statusValue == "" {
		return fmt.Errorf("status value is required")
	}
	if notionVersion == "" {
		return fmt.Errorf("notion_version is required")
	}
	if d.StatusPropertyType != "status" && d.StatusPropertyType != "select" {
		return fmt.Errorf("status_property_type must be 'status' or 'select'")
	}
	return nil
}

func (d DatabaseConfig) ValidateForHabit(notionVersion string) error {
	if d.DataSourceID == "" {
		return fmt.Errorf("data_source_id is required")
	}
	if d.TitlePropertyName == "" {
		return fmt.Errorf("title_property_name is required")
	}
	if notionVersion == "" {
		return fmt.Errorf("notion_version is required")
	}
	return nil
}

func (d DatabaseConfig) StatusForAction(action string) string {
	switch action {
	case "done":
		return d.StatusDone
	case "paused":
		return d.StatusPaused
	case "resume":
		return d.StatusInProgress
	default:
		return ""
	}
}

func defaultDatabases() []DatabaseConfig {
	return []DatabaseConfig{
		{
			Key:                "tasks",
			Name:               "タスク",
			Kind:               DatabaseKindTask,
			Enabled:            true,
			StatusPropertyType: "status",
		},
		{
			Key:                  "habits",
			Name:                 "習慣",
			Kind:                 DatabaseKindHabit,
			Enabled:              true,
			TitlePropertyName:    "名前",
			CheckboxPropertyName: DefaultHabitDays,
		},
	}
}

func normalizeDatabases(dbs []DatabaseConfig) []DatabaseConfig {
	used := make(map[string]struct{}, len(dbs))
	for i := range dbs {
		dbs[i].Key = strings.TrimSpace(dbs[i].Key)
		if dbs[i].Key == "" {
			dbs[i].Key = fmt.Sprintf("db-%d", i+1)
		}
		key := dbs[i].Key
		for {
			if _, ok := used[key]; !ok {
				break
			}
			key = fmt.Sprintf("%s-%d", dbs[i].Key, i+1)
		}
		dbs[i].Key = key
		used[key] = struct{}{}

		dbs[i].Kind = strings.TrimSpace(dbs[i].Kind)
		if dbs[i].Kind == "" {
			dbs[i].Kind = DatabaseKindTask
		}
		dbs[i].Name = strings.TrimSpace(dbs[i].Name)
		if dbs[i].Name == "" {
			dbs[i].Name = defaultNameForKind(dbs[i].Kind)
		}
		if dbs[i].Kind == DatabaseKindHabit {
			if strings.TrimSpace(dbs[i].TitlePropertyName) == "" {
				dbs[i].TitlePropertyName = "名前"
			}
			if strings.TrimSpace(dbs[i].CheckboxPropertyName) == "" {
				dbs[i].CheckboxPropertyName = DefaultHabitDays
			}
		}
	}
	return dbs
}

func defaultNameForKind(kind string) string {
	switch kind {
	case DatabaseKindHabit:
		return "習慣"
	default:
		return "タスク"
	}
}
