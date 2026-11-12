package app

import (
	"fmt"
	"strings"

	"nudge/internal/dto"
)

// TraySummary はメニューバーの件数ラベルに出す内容（詳細設計 5.3）。
type TraySummary struct {
	// Active はトークンがあり、有効な DB が 1 つ以上あるか。
	Active bool
	// Ready は起動後の最初の取得が終わったか。
	Ready bool
	// Reloading は設定の保存後の取り直しの途中か。その間はラベルを変えない。
	Reloading bool
	// HasError は有効な DB のうち、直近の取得に失敗したものがあるか。
	HasError bool
	// DueCount は期限切れと今日のタスクの数（全タスク DB の合計）。
	DueCount int
	// HabitCount は残っている習慣の数（全習慣 DB の合計）。
	HabitCount int
}

// FormatTrayLabel はメニューバーのラベルの文字列を作る。0 の項目は出さない。
func FormatTrayLabel(s TraySummary) string {
	if !s.Active || !s.Ready {
		return ""
	}
	if s.HasError {
		return "!"
	}
	parts := make([]string, 0, 2)
	if s.DueCount > 0 {
		parts = append(parts, fmt.Sprintf("⚠%d", s.DueCount))
	}
	if s.HabitCount > 0 {
		parts = append(parts, fmt.Sprintf("✓%d", s.HabitCount))
	}
	return strings.Join(parts, " ")
}

// TraySummary は今のキャッシュと取得結果から件数をまとめる。日付は呼ばれた時点で判定する。
func (a *App) TraySummary() TraySummary {
	cfg := a.currentConfig()
	summary := TraySummary{Active: a.hasToken() && hasEnabledDatabase(cfg)}

	a.cacheMu.Lock()
	summary.Ready = a.ready
	summary.Reloading = a.reloading
	tasks := make(map[string][]dto.Task, len(a.taskCache))
	for k, v := range a.taskCache {
		tasks[k] = cloneTasks(v)
	}
	habits := make(map[string][]dto.Task, len(a.habitCache))
	for k, v := range a.habitCache {
		habits[k] = cloneTasks(v)
	}
	states := make(map[string]syncRecord, len(a.syncStates))
	for k, v := range a.syncStates {
		states[k] = v
	}
	a.cacheMu.Unlock()

	now := a.localNow()
	for _, db := range cfg.Databases {
		if !db.Enabled {
			continue
		}
		if rec := states[db.Key]; rec.attempted && !rec.ok {
			summary.HasError = true
		}
		if db.Kind == dto.DatabaseKindHabit {
			summary.HabitCount += countRemainingHabits(habits[db.Key])
			continue
		}
		summary.DueCount += countDueTasks(buildTaskView(tasks[db.Key], db, now))
	}
	return summary
}

// countRemainingHabits は残っている習慣の数を返す。キャッシュは今日の未チェックの一覧なので、その件数を数える。
// 週の目標に届いた習慣を除く判定は、習慣の週集計（段階 3）で入れる。
func countRemainingHabits(today []dto.Task) int {
	return len(today)
}

func (a *App) hasToken() bool {
	token, err := a.tokenStore.GetToken()
	return err == nil && strings.TrimSpace(token) != ""
}

func hasEnabledDatabase(cfg dto.Config) bool {
	for _, db := range cfg.Databases {
		if db.Enabled {
			return true
		}
	}
	return false
}

// SetSummaryNotifier は件数が変わりうるときに合図を送る先を登録する（起動時に 1 回）。
// 容量 1 の channel を渡す。合図は値を持たず、受け取った側が最新の件数を計算し直す。
func (a *App) SetSummaryNotifier(ch chan<- struct{}) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.notifier = ch
}

// notifySummary は合図を送る。すでに 1 つ溜まっていれば捨てる（ブロックしない）。
// cacheMu・refreshMu を持たない状態で呼ぶ。
func (a *App) notifySummary() {
	a.cacheMu.Lock()
	ch := a.notifier
	a.cacheMu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}
