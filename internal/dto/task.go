package dto

// Task は UI に渡す最小単位のタスク情報。
type Task struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	LastEditedTime string `json:"last_edited_time"`
	Checked        bool   `json:"checked"`

	// v2: 期日（ローカル日付 YYYY-MM-DD、無ければ空）と優先度（Select の名前）。
	DueDate  string `json:"due_date"`
	Priority string `json:"priority"`
	// v2: 表示のまとまり（overdue / today / in_progress / upcoming）と、進行中かどうか。
	// 期限切れ・今日のまとまりには進行中と未着手が混ざるので、「開始」を出すかは IsInProgress で決める。
	Group        string `json:"group"`
	IsInProgress bool   `json:"is_in_progress"`
	// v2: 習慣のときだけ値が入る（at_risk / unreachable / 空）。
	Risk string `json:"risk"`
}
