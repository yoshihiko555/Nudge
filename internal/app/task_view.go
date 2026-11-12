package app

import (
	"sort"
	"time"

	"nudge/internal/dto"
)

// タスクのまとまり（詳細設計 3 章）。
const (
	TaskGroupOverdue    = "overdue"
	TaskGroupToday      = "today"
	TaskGroupInProgress = "in_progress"
	TaskGroupUpcoming   = "upcoming"
)

// upcomingDays は「直近 7 日」の範囲（明日〜今日＋7 日）。
const upcomingDays = 7

const dateLayout = "2006-01-02"

var taskGroupOrder = map[string]int{
	TaskGroupOverdue:    0,
	TaskGroupToday:      1,
	TaskGroupInProgress: 2,
	TaskGroupUpcoming:   3,
}

// buildTaskView は取得したタスクにまとまりを付けて並べ替える。どのまとまりにも入らないタスクは捨てる。
// 振り分けは呼ばれた時点の now を基準にするので、0 時をまたいでも次の表示から切り替わる。
// now は判定に使うタイムゾーンに直してから渡す（App.localNow）。
func buildTaskView(tasks []dto.Task, db dto.DatabaseConfig, now time.Time) []dto.Task {
	if !db.UsesDueDateQuery() {
		return buildLegacyTaskView(tasks)
	}
	today := now.Format(dateLayout)
	upcomingUntil := now.AddDate(0, 0, upcomingDays).Format(dateLayout)
	ranks := priorityRanks(db.PriorityOrder)

	out := make([]dto.Task, 0, len(tasks))
	for _, task := range tasks {
		task.IsInProgress = task.Status == db.StatusInProgress
		task.Group = taskGroup(task, today, upcomingUntil)
		if task.Group == "" {
			continue
		}
		out = append(out, task)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return lessTask(out[i], out[j], ranks)
	})
	return out
}

// buildLegacyTaskView は v1 の問い合わせ（進行中のみ）の結果を、v1 と同じ並び（最終編集の新しい順）で返す。
func buildLegacyTaskView(tasks []dto.Task) []dto.Task {
	out := make([]dto.Task, 0, len(tasks))
	for _, task := range tasks {
		task.Group = TaskGroupInProgress
		task.IsInProgress = true
		out = append(out, task)
	}
	return out
}

func taskGroup(task dto.Task, today, upcomingUntil string) string {
	due := task.DueDate
	switch {
	case due != "" && due < today:
		return TaskGroupOverdue
	case due != "" && due == today:
		return TaskGroupToday
	case task.IsInProgress:
		return TaskGroupInProgress
	case due != "" && due <= upcomingUntil:
		return TaskGroupUpcoming
	default:
		return ""
	}
}

// lessTask はまとまりの順に並べ、まとまりの中は詳細設計 3 章の表のキーで並べる。
// 最終編集の新しい順は、RFC 3339 の文字列の比較で決める。
func lessTask(a, b dto.Task, ranks map[string]int) bool {
	if ga, gb := taskGroupOrder[a.Group], taskGroupOrder[b.Group]; ga != gb {
		return ga < gb
	}
	if sortsByDueDate(a.Group) && a.DueDate != b.DueDate {
		return a.DueDate < b.DueDate
	}
	if pa, pb := priorityRank(a.Priority, ranks), priorityRank(b.Priority, ranks); pa != pb {
		return pa < pb
	}
	return a.LastEditedTime > b.LastEditedTime
}

func sortsByDueDate(group string) bool {
	return group == TaskGroupOverdue || group == TaskGroupUpcoming
}

// priorityRanks は優先度の値から順位（0 が最上位）への対応を作る。
func priorityRanks(order []string) map[string]int {
	ranks := make(map[string]int, len(order))
	for i, v := range order {
		ranks[v] = i
	}
	return ranks
}

// priorityRank は優先度の順位を返す。順に無い値と空は最後（len(order)）にする。
func priorityRank(priority string, ranks map[string]int) int {
	if rank, ok := ranks[priority]; ok {
		return rank
	}
	return len(ranks)
}

// countDueTasks は件数（⚠）の対象になる、期限切れと今日のタスクの数を返す。
func countDueTasks(view []dto.Task) int {
	count := 0
	for _, task := range view {
		if task.Group == TaskGroupOverdue || task.Group == TaskGroupToday {
			count++
		}
	}
	return count
}
