package dto

import (
	"fmt"
	"strings"
)

const (
	DatabaseKindTask  = "task"
	DatabaseKindHabit = "habit"
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
	// v2: 期日・優先度（タスク DB）とゴール（習慣 DB）。空なら該当機能は無効。
	DueDatePropertyName  string   `json:"due_date_property_name"`
	PriorityPropertyName string   `json:"priority_property_name"`
	PriorityOrder        []string `json:"priority_order"`
	GoalPropertyName     string   `json:"goal_property_name"`
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
	return Config{
		PollIntervalSeconds: 60,
		MaxResults:          30,
	}
}

func (c Config) Normalize() Config {
	if len(c.Databases) > 0 {
		c.Databases = normalizeDatabases(c.Databases)
	}
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
		return invalidConfig("data_source_id is required")
	}
	if d.TitlePropertyName == "" {
		return invalidConfig("title_property_name is required")
	}
	if d.StatusPropertyName == "" {
		return invalidConfig("status_property_name is required")
	}
	if statusValue == "" {
		return invalidConfig("status value is required")
	}
	if d.StatusPropertyType != "status" && d.StatusPropertyType != "select" {
		return invalidConfig("status_property_type must be 'status' or 'select'")
	}
	return nil
}

func (d DatabaseConfig) ValidateForHabit(notionVersion string) error {
	if d.DataSourceID == "" {
		return invalidConfig("data_source_id is required")
	}
	if d.TitlePropertyName == "" {
		return invalidConfig("title_property_name is required")
	}
	return nil
}

// UsesDueDateQuery は、期日つきの問い合わせ（進行中 または 期日が近い未完了）を使える設定かを返す。
// 期日のプロパティと進行中・完了・中断の値がすべてそろっていないと「完了でない」の条件を組めないため、
// そろわない DB は v1 と同じ「進行中のみ」の問い合わせにする。
func (d DatabaseConfig) UsesDueDateQuery() bool {
	return d.DueDatePropertyName != "" &&
		d.StatusInProgress != "" &&
		d.StatusDone != "" &&
		d.StatusPaused != ""
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

func normalizeDatabases(dbs []DatabaseConfig) []DatabaseConfig {
	used := make(map[string]struct{}, len(dbs))
	for i := range dbs {
		dbs[i].Key = strings.TrimSpace(dbs[i].Key)
		if dbs[i].Key == "" {
			dbs[i].Key = fmt.Sprintf("db-%d", i+1)
		}
		key := dbs[i].Key
		counter := 2
		for {
			if _, ok := used[key]; !ok {
				break
			}
			key = fmt.Sprintf("%s-%d", dbs[i].Key, counter)
			counter++
		}
		dbs[i].Key = key
		used[key] = struct{}{}

		dbs[i].Kind = strings.TrimSpace(dbs[i].Kind)
		if dbs[i].Kind == "" {
			dbs[i].Kind = DatabaseKindTask
		}
		dbs[i].Name = strings.TrimSpace(dbs[i].Name)
		dbs[i] = normalizeV2Fields(dbs[i])
	}
	return dbs
}

// normalizeV2Fields は v2 の設定キーの空白を除き、優先度の順から空と重複を除く。
// 優先度のプロパティが空なら、使われない優先度の順も空にする。既定値は補わない。
func normalizeV2Fields(db DatabaseConfig) DatabaseConfig {
	db.DueDatePropertyName = strings.TrimSpace(db.DueDatePropertyName)
	db.PriorityPropertyName = strings.TrimSpace(db.PriorityPropertyName)
	db.GoalPropertyName = strings.TrimSpace(db.GoalPropertyName)
	if db.PriorityPropertyName == "" {
		db.PriorityOrder = nil
		return db
	}
	db.PriorityOrder = uniqueTrimmed(db.PriorityOrder)
	return db
}

func uniqueTrimmed(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
