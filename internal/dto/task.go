package dto

// Task は UI に渡す最小単位のタスク情報。
type Task struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	LastEditedTime string `json:"last_edited_time"`
}
