package app

import (
	"fmt"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

// habitWeekRecord は今週の習慣のページ。loaded は一度でも取得に成功したか。
type habitWeekRecord struct {
	loaded bool
	pages  []notion.HabitPage
}

// usesWeeklyHabits は曜日チェック列が日〜土の 7 列そろっていて、週集計を使えるかを返す。
// そろっていない DB は v1 と同じ処理（同名の最新 1 件・今日の列だけ）にする（NFR-02）。
func usesWeeklyHabits(db dto.DatabaseConfig) bool {
	return len(weekdayColumns(db)) == notion.WeekdayCount
}

func weekdayColumns(db dto.DatabaseConfig) []string {
	if db.CheckboxPropertyName == "" {
		return nil
	}
	return splitAndTrim(db.CheckboxPropertyName, ",")
}

// habitSummaryFor はキャッシュを写し取ってから、今の日付で週集計を作る。
func (a *App) habitSummaryFor(db dto.DatabaseConfig) habitSummaryResult {
	a.cacheMu.Lock()
	week := a.habitWeek[db.Key]
	history := a.historyCache[db.Key]
	weekPages := append([]notion.HabitPage(nil), week.pages...)
	historyPages := append([]notion.HabitPage(nil), history.pages...)
	a.cacheMu.Unlock()
	result := buildHabitSummary(weekPages, week.loaded, historyPages, history.loaded, a.localNow())
	// 過去分を一度も取れないまま失敗しているなら「読み込み中」ではなく、連続達成週数を「—」にする。
	if !history.loaded && history.attempted && !history.ok {
		result.Summary.HistoryLoading = false
	}
	return result
}

// todayHabits は今日の未チェックの習慣の一覧を返す。まだ取得できていなければ false。
func (a *App) todayHabits(databaseKey string) ([]dto.Task, bool) {
	db, ok := a.currentConfig().DatabaseByKey(databaseKey)
	if !ok || !usesWeeklyHabits(db) {
		return a.getHabitCache(databaseKey)
	}
	a.cacheMu.Lock()
	loaded := a.habitWeek[databaseKey].loaded
	a.cacheMu.Unlock()
	if !loaded {
		return nil, false
	}
	today := a.habitSummaryFor(db).Today
	if today == nil {
		today = []dto.Task{}
	}
	return today, true
}

// GetHabitSummary は習慣の週集計を返す（API-003）。キャッシュだけを読み、取得はしない。
func (a *App) GetHabitSummary(databaseKey string) (dto.HabitSummary, error) {
	db, ok := a.currentConfig().DatabaseByKey(databaseKey)
	if !ok {
		return dto.HabitSummary{}, fmt.Errorf("database not found")
	}
	if db.Kind != dto.DatabaseKindHabit {
		return dto.HabitSummary{}, fmt.Errorf("database kind is not habit")
	}
	if !usesWeeklyHabits(db) {
		return dto.HabitSummary{
			Enabled:    false,
			Reason:     dto.HabitSummaryReasonWeekdayColumns,
			TodayIndex: int(a.localNow().Weekday()),
			Rows:       []dto.HabitWeekRow{},
		}, nil
	}
	return a.habitSummaryFor(db).Summary, nil
}

// remainingHabitCount は件数（✓）に数える残っている習慣の数を返す（詳細設計 4 章 6）。
// 週集計を使わない DB は数えない。
func (a *App) remainingHabitCount(db dto.DatabaseConfig) int {
	if !usesWeeklyHabits(db) {
		return 0
	}
	return a.habitSummaryFor(db).Remaining
}

// historySyncState は過去分の取得の状態を、取得結果（API-006）の形にして返す。
func historySyncState(db dto.DatabaseConfig, rec historyRecord) (ok *bool, fetchedFor string, errorKind string) {
	if db.Kind != dto.DatabaseKindHabit || !usesWeeklyHabits(db) || !rec.attempted {
		return nil, "", ""
	}
	value := rec.ok
	return &value, rec.fetchedFor, rec.errorKind
}
