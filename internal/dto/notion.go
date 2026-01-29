package dto

// DatabaseProperty は Notion データベースのプロパティ情報。
type DatabaseProperty struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Options []string `json:"options,omitempty"`
}
