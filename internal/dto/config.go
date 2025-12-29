package dto

import "fmt"

// Config はローカル設定ファイルの内容。
type Config struct {
	DatabaseID          string `json:"database_id"`
	DataSourceID        string `json:"data_source_id"`
	TitlePropertyName   string `json:"title_property_name"`
	StatusPropertyName  string `json:"status_property_name"`
	StatusPropertyType  string `json:"status_property_type"` // "status" or "select"
	StatusInProgress    string `json:"status_in_progress"`
	StatusDone          string `json:"status_done"`
	StatusPaused        string `json:"status_paused"`
	PollIntervalSeconds int    `json:"poll_interval_seconds"`
	MaxResults          int    `json:"max_results"`
	NotionVersion       string `json:"notion_version"`
}

func DefaultConfig() Config {
	return Config{
		PollIntervalSeconds: 60,
		MaxResults:          30,
		StatusPropertyType:  "status",
	}
}

func (c Config) ValidateForStatusQuery(statusValue string) error {
	if c.DataSourceID == "" {
		return fmt.Errorf("data_source_id is required")
	}
	if c.TitlePropertyName == "" {
		return fmt.Errorf("title_property_name is required")
	}
	if c.StatusPropertyName == "" {
		return fmt.Errorf("status_property_name is required")
	}
	if statusValue == "" {
		return fmt.Errorf("status value is required")
	}
	if c.NotionVersion == "" {
		return fmt.Errorf("notion_version is required")
	}
	if c.StatusPropertyType != "status" && c.StatusPropertyType != "select" {
		return fmt.Errorf("status_property_type must be 'status' or 'select'")
	}
	return nil
}

func (c Config) ValidateForQuery() error {
	return c.ValidateForStatusQuery(c.StatusInProgress)
}

func (c Config) ValidateForUpdate() error {
	if err := c.ValidateForStatusQuery(c.StatusInProgress); err != nil {
		return err
	}
	if c.StatusDone == "" {
		return fmt.Errorf("status_done is required")
	}
	if c.StatusPaused == "" {
		return fmt.Errorf("status_paused is required")
	}
	return nil
}
