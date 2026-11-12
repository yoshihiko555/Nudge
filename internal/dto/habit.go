package dto

// 危ない習慣の区分（詳細設計 4 章 4）。
const (
	HabitRiskAtRisk      = "at_risk"
	HabitRiskUnreachable = "unreachable"
)

// 週集計を出さない・出せない理由（API-003）。
const (
	HabitSummaryReasonWeekdayColumns = "weekday_columns"
	HabitSummaryReasonNoWeekPages    = "no_week_pages"
	HabitSummaryReasonNotLoaded      = "not_loaded"
)

// HabitSummary は習慣の週集計（API-003 の応答）。
type HabitSummary struct {
	Enabled        bool           `json:"enabled"`
	Reason         string         `json:"reason"`
	TodayIndex     int            `json:"today_index"`
	HistoryLoading bool           `json:"history_loading"`
	Rows           []HabitWeekRow `json:"rows"`
}

// HabitWeekRow は 1 つの習慣の今週の状況。
type HabitWeekRow struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	Days         []bool `json:"days"`
	CheckedCount int    `json:"checked_count"`
	// Goal は週の目標回数。読めなければ nil。
	Goal *int `json:"goal"`
	// Streak は連続達成週数。目標が読めないか、過去分が一度も取れていなければ nil。
	Streak       *int   `json:"streak"`
	StreakCapped bool   `json:"streak_capped"`
	Risk         string `json:"risk"`
}
