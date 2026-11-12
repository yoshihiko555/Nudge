package app

import (
	"reflect"
	"testing"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

func TestParseGoal(t *testing.T) {
	t.Parallel()

	three := 3
	one := 1
	tests := []struct {
		in   string
		want *int
	}{
		{in: "週１回", want: &one},
		{in: "週1回", want: &one},
		{in: " 週 3 回 ", want: &three},
		{in: "週３回", want: &three},
		{in: "毎日", want: nil},
		{in: "週0回", want: nil},
		{in: "週8回", want: nil},
		{in: "", want: nil},
	}
	for _, tt := range tests {
		got := parseGoal(tt.in)
		if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
			t.Fatalf("parseGoal(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestWeekStart_Boundaries(t *testing.T) {
	t.Parallel()

	sat := time.Date(2026, 11, 14, 23, 59, 0, 0, jst)
	sun := time.Date(2026, 11, 15, 0, 0, 0, 0, jst)
	if got := weekStart(sat); !got.Equal(time.Date(2026, 11, 8, 0, 0, 0, 0, jst)) {
		t.Fatalf("weekStart(土曜 23:59) = %v", got)
	}
	if got := weekStart(sun); !got.Equal(sun) {
		t.Fatalf("weekStart(日曜 0:00) = %v", got)
	}
}

func TestWeekStart_DSTKeepsMidnight(t *testing.T) {
	t.Parallel()

	la, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skipf("tzdata not available: %v", err)
	}
	// 2026-11-01（日）に夏時間が終わる週。
	ws := weekStart(time.Date(2026, 11, 4, 12, 0, 0, 0, la))
	if ws.Day() != 1 || ws.Hour() != 0 {
		t.Fatalf("weekStart across DST = %v, want 11/1 00:00", ws)
	}
	prev := ws.AddDate(0, 0, -7)
	if prev.Day() != 25 || prev.Hour() != 0 {
		t.Fatalf("previous week start = %v, want 10/25 00:00", prev)
	}
}

// 今日は 2026-11-12（木、today_index 4）。今週は 11/8（日）から。
var summaryNow = time.Date(2026, 11, 12, 9, 0, 0, 0, jst)

func habitPage(title string, weekOf time.Time, goal string, checked ...int) notion.HabitPage {
	p := notion.HabitPage{ID: title + weekOf.Format("0102"), Title: title, CreatedTime: weekOf.Add(30 * time.Minute), Goal: goal}
	for _, i := range checked {
		p.Days[i] = true
	}
	return p
}

func week(k int) time.Time {
	return time.Date(2026, 11, 8, 0, 0, 0, 0, jst).AddDate(0, 0, -7*k)
}

func rowByName(t *testing.T, s dto.HabitSummary, name string) dto.HabitWeekRow {
	t.Helper()
	for _, r := range s.Rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("row %q not found in %+v", name, s.Rows)
	return dto.HabitWeekRow{}
}

func TestHabitSummary_ReasonsWhenNoData(t *testing.T) {
	t.Parallel()

	if got := buildHabitSummary(nil, false, nil, false, summaryNow).Summary; got.Reason != dto.HabitSummaryReasonNotLoaded {
		t.Fatalf("reason = %q, want not_loaded", got.Reason)
	}
	// 先週のページしか無い（今週分がまだ作られていない）なら、今週分として使わない。
	lastWeekOnly := []notion.HabitPage{habitPage("掃除", week(1), "週２回")}
	got := buildHabitSummary(lastWeekOnly, true, nil, true, summaryNow)
	if got.Summary.Reason != dto.HabitSummaryReasonNoWeekPages || len(got.Today) != 0 {
		t.Fatalf("summary = %+v today=%v, want no_week_pages and empty today", got.Summary, got.Today)
	}
}

func TestHabitSummary_DuplicateInSameWeekUsesLatest(t *testing.T) {
	t.Parallel()

	older := habitPage("掃除", week(0), "週２回")
	newer := habitPage("掃除", week(0), "週２回", 0)
	newer.ID = "newer"
	newer.CreatedTime = older.CreatedTime.Add(time.Hour)
	got := buildHabitSummary([]notion.HabitPage{older, newer}, true, nil, true, summaryNow).Summary
	if len(got.Rows) != 1 || got.Rows[0].CheckedCount != 1 {
		t.Fatalf("rows = %+v, want the newer page only", got.Rows)
	}
	if got.TodayIndex != 4 || !reflect.DeepEqual(got.Rows[0].Days, []bool{true, false, false, false, false, false, false}) {
		t.Fatalf("today_index=%d days=%v", got.TodayIndex, got.Rows[0].Days)
	}
}

func TestStreak_SkipsMissingAndUnreadableWeeksAndStopsAtMiss(t *testing.T) {
	t.Parallel()

	current := []notion.HabitPage{habitPage("ストレッチ", week(0), "週２回", 0)}
	history := []notion.HabitPage{
		habitPage("ストレッチ", week(1), "週２回", 0, 1), // 達成
		// week(2) はページ無し → 飛ばす
		habitPage("ストレッチ", week(3), "毎日", 0),        // ゴールが読めない → 飛ばす
		habitPage("ストレッチ", week(4), "週２回", 0, 1, 2), // 達成
		habitPage("ストレッチ", week(5), "週２回", 0),       // 未達 → ここで止まる
		habitPage("ストレッチ", week(6), "週２回", 0, 1),    // 数えない
	}
	row := rowByName(t, buildHabitSummary(current, true, history, true, summaryNow).Summary, "ストレッチ")
	if row.Streak == nil || *row.Streak != 2 || row.StreakCapped {
		t.Fatalf("streak = %v capped=%v, want 2", row.Streak, row.StreakCapped)
	}
}

func TestStreak_ThisWeekAchievedAddsOne(t *testing.T) {
	t.Parallel()

	current := []notion.HabitPage{habitPage("薬", week(0), "週２回", 0, 1)}
	history := []notion.HabitPage{habitPage("薬", week(1), "週２回", 0, 1)}
	row := rowByName(t, buildHabitSummary(current, true, history, true, summaryNow).Summary, "薬")
	if row.Streak == nil || *row.Streak != 2 {
		t.Fatalf("streak = %v, want 2 (last week + this week)", row.Streak)
	}
}

func TestStreak_CappedOnlyWhenOldestWeekExists(t *testing.T) {
	t.Parallel()

	current := []notion.HabitPage{habitPage("サプリ", week(0), "週１回")}
	full := make([]notion.HabitPage, 0, historyWeeks)
	for k := 1; k <= historyWeeks; k++ {
		full = append(full, habitPage("サプリ", week(k), "週１回", 0))
	}
	row := rowByName(t, buildHabitSummary(current, true, full, true, summaryNow).Summary, "サプリ")
	if !row.StreakCapped || row.Streak == nil || *row.Streak != historyWeeks {
		t.Fatalf("streak=%v capped=%v, want 52 capped", row.Streak, row.StreakCapped)
	}

	// 履歴が短い（52 週前のページが無い）なら、途切れていなくても capped にしない。
	short := full[:10]
	row = rowByName(t, buildHabitSummary(current, true, short, true, summaryNow).Summary, "サプリ")
	if row.StreakCapped || row.Streak == nil || *row.Streak != 10 {
		t.Fatalf("streak=%v capped=%v, want 10 not capped", row.Streak, row.StreakCapped)
	}
}

func TestStreak_NilWhenGoalUnreadableOrHistoryNotLoaded(t *testing.T) {
	t.Parallel()

	current := []notion.HabitPage{habitPage("学習", week(0), "毎日"), habitPage("掃除", week(0), "週２回")}
	s := buildHabitSummary(current, true, nil, true, summaryNow).Summary
	if row := rowByName(t, s, "学習"); row.Goal != nil || row.Streak != nil {
		t.Fatalf("unreadable goal must hide goal and streak: %+v", row)
	}
	s = buildHabitSummary(current, true, nil, false, summaryNow).Summary
	if row := rowByName(t, s, "掃除"); row.Streak != nil || !s.HistoryLoading {
		t.Fatalf("streak must be nil while history is not loaded: %+v loading=%v", row, s.HistoryLoading)
	}
}

func TestRisk(t *testing.T) {
	t.Parallel()

	// 木曜（today_index 4）なので、今日を含む残り日数 D = 3（木・金・土）。
	pages := []notion.HabitPage{
		habitPage("ぎりぎり", week(0), "週５回", 0, 1),    // R = 3 = D → at_risk
		habitPage("もう無理", week(0), "週５回", 0),       // R = 4 > D → unreachable
		habitPage("余裕", week(0), "週３回", 0, 1),      // R = 1 < D → 空
		habitPage("今日済み", week(0), "週７回", 0, 1, 4), // 今日チェック済み → 空
		habitPage("達成済み", week(0), "週２回", 0, 1),    // R = 0 → 空
		habitPage("目標不明", week(0), "", 0),          // ゴールが読めない → 空
	}
	res := buildHabitSummary(pages, true, nil, true, summaryNow)
	want := map[string]string{
		"ぎりぎり": dto.HabitRiskAtRisk,
		"もう無理": dto.HabitRiskUnreachable,
		"余裕":   "",
		"今日済み": "",
		"達成済み": "",
		"目標不明": "",
	}
	for name, risk := range want {
		if got := rowByName(t, res.Summary, name).Risk; got != risk {
			t.Fatalf("%s risk = %q, want %q", name, got, risk)
		}
	}
	// 今日の一覧は今日が未チェックのもの。件数（✓）は、そのうち目標に届いていないもの（目標不明も数える）。
	if len(res.Today) != 5 {
		t.Fatalf("today = %d, want 5", len(res.Today))
	}
	if res.Remaining != 4 {
		t.Fatalf("remaining = %d, want 4 (達成済みを除く)", res.Remaining)
	}
	for _, task := range res.Today {
		if task.Title == "ぎりぎり" && task.Risk != dto.HabitRiskAtRisk {
			t.Fatalf("today list must carry risk: %+v", task)
		}
	}
}
