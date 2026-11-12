package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

// fakeNotion はタスク・今週の習慣・過去の習慣の問い合わせを見分けて応え、それぞれの回数を数える。
type fakeNotion struct {
	mu          sync.Mutex
	taskQueries int
	weekQueries int
	histQueries int
	patches     int
	weekBodies  []map[string]any
	histBodies  []map[string]any
	failHistory atomic.Bool
	weekPages   []string
	histPages   []string
}

func habitPageJSON(id, title, created, goal string, checked ...int) string {
	days := []string{"日", "月", "火", "水", "木", "金", "土"}
	props := []string{fmt.Sprintf(`"名前":{"type":"title","title":[{"plain_text":%q}]}`, title)}
	for i, d := range days {
		on := false
		for _, c := range checked {
			if c == i {
				on = true
			}
		}
		props = append(props, fmt.Sprintf(`%q:{"type":"checkbox","checkbox":%v}`, d, on))
	}
	props = append(props, fmt.Sprintf(`"ゴール":{"type":"select","select":{"name":%q}}`, goal))
	return fmt.Sprintf(`{"id":%q,"url":"https://example.com/%s","created_time":%q,"last_edited_time":%q,"properties":{%s}}`,
		id, id, created, created, strings.Join(props, ","))
}

func (f *fakeNotion) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodPatch {
			f.patches++
			_, _ = w.Write([]byte(`{}`))
			return
		}
		filter, _ := body["filter"].(map[string]any)
		switch {
		case filter["or"] != nil || filter["property"] != nil:
			f.taskQueries++
			_, _ = w.Write([]byte(oneTaskResponse))
		case filter["and"] != nil:
			f.histQueries++
			f.histBodies = append(f.histBodies, body)
			if f.failHistory.Load() {
				http.Error(w, `{"code":"internal_server_error"}`, http.StatusInternalServerError)
				return
			}
			fmt.Fprintf(w, `{"results":[%s],"has_more":false}`, strings.Join(f.histPages, ","))
		default:
			f.weekQueries++
			f.weekBodies = append(f.weekBodies, body)
			fmt.Fprintf(w, `{"results":[%s],"has_more":false}`, strings.Join(f.weekPages, ","))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func (f *fakeNotion) counts() (task, week, hist int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.taskQueries, f.weekQueries, f.histQueries
}

func weeklyHabitDB() dto.DatabaseConfig {
	return dto.DatabaseConfig{
		Key:                  "habits",
		Name:                 "習慣",
		Kind:                 dto.DatabaseKindHabit,
		Enabled:              true,
		DatabaseID:           "hdb",
		DataSourceID:         "hds",
		TitlePropertyName:    "名前",
		CheckboxPropertyName: "日,月,火,水,木,金,土",
		GoalPropertyName:     "ゴール",
	}
}

// newWeeklyApp は時計を 2026-11-12（木）9:00 JST に固定し、過去分の取得を同期で走らせる。
func newWeeklyApp(t *testing.T, url string, clock *time.Time) *App {
	t.Helper()
	cfg := dto.Config{Databases: []dto.DatabaseConfig{legacyTaskDB(), weeklyHabitDB()}, MaxResults: 30}
	client := notion.NewClient(&stubTokenStore{token: "test"}, notion.WithBaseURL(url), notion.WithRetry(0, 0))
	a := NewApp(&stubConfigStore{cfg: cfg}, &stubTokenStore{token: "test"}, client)
	if _, err := a.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	a.now = func() time.Time { return *clock }
	a.loc = jst
	a.spawn = func(f func()) { f() }
	return a
}

func standardHabitFixture() *fakeNotion {
	return &fakeNotion{
		weekPages: []string{
			habitPageJSON("w-stretch", "ストレッチ", "2026-11-07T15:31:00.000Z", "週５回", 0, 1),
			habitPageJSON("w-clean", "掃除", "2026-11-07T15:31:00.000Z", "週２回", 3),
		},
		histPages: []string{
			habitPageJSON("h-stretch-1", "ストレッチ", "2026-10-31T15:04:00.000Z", "週５回", 0, 1, 2, 3, 4),
			habitPageJSON("h-clean-1", "掃除", "2026-10-31T15:04:00.000Z", "週２回", 0),
		},
	}
}

func TestWeeklyHabitFlow(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	ctx := context.Background()

	if err := a.refreshAll(ctx); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	// 今週分は作成日時 ≧ 今週の日曜 0:00（JST）、過去分は 52 週前〜今週の日曜で問い合わせる。
	weekFilter := fake.weekBodies[0]["filter"].(map[string]any)["created_time"].(map[string]any)
	if weekFilter["on_or_after"] != "2026-11-08T00:00:00+09:00" {
		t.Fatalf("week filter = %v", weekFilter)
	}
	histFilter := fake.histBodies[0]["filter"].(map[string]any)["and"].([]any)
	from := histFilter[0].(map[string]any)["created_time"].(map[string]any)["on_or_after"]
	before := histFilter[1].(map[string]any)["created_time"].(map[string]any)["before"]
	if from != "2025-11-09T00:00:00+09:00" || before != "2026-11-08T00:00:00+09:00" {
		t.Fatalf("history range = %v .. %v", from, before)
	}

	today, err := a.GetHabits(ctx, "habits", false)
	if err != nil {
		t.Fatalf("GetHabits: %v", err)
	}
	if len(today) != 2 {
		t.Fatalf("today = %+v, want 2 unchecked habits", today)
	}
	summary, err := a.GetHabitSummary("habits")
	if err != nil {
		t.Fatalf("GetHabitSummary: %v", err)
	}
	stretch := rowByName(t, summary, "ストレッチ")
	if stretch.Goal == nil || *stretch.Goal != 5 || stretch.Streak == nil || *stretch.Streak != 1 || stretch.Risk != dto.HabitRiskAtRisk {
		t.Fatalf("stretch row = %+v (streak=%v)", stretch, stretch.Streak)
	}
	clean := rowByName(t, summary, "掃除")
	if clean.Streak == nil || *clean.Streak != 0 {
		t.Fatalf("clean streak = %v, want 0 (last week missed)", clean.Streak)
	}
	states := a.GetSyncState()
	for _, s := range states {
		if s.DatabaseKey == "habits" && (s.HistoryOK == nil || !*s.HistoryOK || s.HistoryFetchedFor != "2026-11-12") {
			t.Fatalf("habit sync state = %+v", s)
		}
		if s.DatabaseKey == "tasks" && s.HistoryOK != nil {
			t.Fatalf("task DB must not have history state: %+v", s)
		}
	}
	if got := a.TraySummary(); got.HabitCount != 2 {
		t.Fatalf("tray habit count = %d, want 2", got.HabitCount)
	}
}

func TestHistory_FetchesOncePerDayAndOnDateChange(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := a.refreshAll(ctx); err != nil {
			t.Fatalf("refreshAll: %v", err)
		}
	}
	if _, _, hist := fake.counts(); hist != 1 {
		t.Fatalf("history queries = %d, want 1 on the same day", hist)
	}
	clock = clock.AddDate(0, 0, 1)
	if err := a.refreshAll(ctx); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	if _, _, hist := fake.counts(); hist != 2 {
		t.Fatalf("history queries = %d, want 2 after the date changed", hist)
	}
}

func TestHistory_ManualRefreshForcesAndChecksDoNot(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	ctx := context.Background()
	if err := a.refreshAll(ctx); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}

	// 習慣のチェックの後の再取得（force だけ）では過去分を取らない。
	if err := a.UpdateHabitCheck(ctx, "habits", "w-clean", true); err != nil {
		t.Fatalf("UpdateHabitCheck: %v", err)
	}
	if _, err := a.GetHabits(ctx, "habits", true); err != nil {
		t.Fatalf("GetHabits: %v", err)
	}
	if _, _, hist := fake.counts(); hist != 1 {
		t.Fatalf("history queries = %d, want 1 (no refetch after a check)", hist)
	}
	// 手動更新（refresh_history）なら強制で取り直す。
	a.RequestHistoryRefresh()
	if _, _, hist := fake.counts(); hist != 2 {
		t.Fatalf("history queries = %d, want 2 after manual refresh", hist)
	}
}

func TestHistory_PendingForceRerunsAfterCurrentRun(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	a.historyMu.Lock()
	a.history.running = true
	a.history.fetchedFor = "2026-11-12"
	a.historyMu.Unlock()

	a.requestHistory(true) // 実行中なので畳み込まれる
	if _, _, hist := fake.counts(); hist != 0 {
		t.Fatalf("history queries = %d, want 0 while running", hist)
	}
	a.finishHistoryFetch("2026-11-12", true, false) // 今の実行が終わった
	if _, _, hist := fake.counts(); hist != 1 {
		t.Fatalf("history queries = %d, want 1 forced rerun", hist)
	}
}

func TestHistory_RetryBackoffAndKeepsCache(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	ctx := context.Background()
	if err := a.refreshAll(ctx); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	fake.failHistory.Store(true)
	clock = clock.AddDate(0, 0, 1) // 翌日。過去分を取り直すが失敗する

	waits := []time.Duration{5 * time.Minute, 15 * time.Minute, 60 * time.Minute, 60 * time.Minute}
	for i, wait := range waits {
		if err := a.refreshAll(ctx); err != nil {
			t.Fatalf("refreshAll: %v", err)
		}
		_, _, hist := fake.counts()
		if want := 2 + i; hist != want {
			t.Fatalf("step %d: history queries = %d, want %d", i, hist, want)
		}
		// 待ち時間の前は取り直さない。
		clock = clock.Add(wait - time.Second)
		if err := a.refreshAll(ctx); err != nil {
			t.Fatalf("refreshAll: %v", err)
		}
		if _, _, h := fake.counts(); h != hist {
			t.Fatalf("step %d: retried before %v", i, wait)
		}
		clock = clock.Add(time.Second)
	}
	// 失敗しても前のキャッシュで連続達成週数を出し、取得結果に失敗が載る。
	summary, _ := a.GetHabitSummary("habits")
	if row := rowByName(t, summary, "ストレッチ"); row.Streak == nil {
		t.Fatalf("streak must use the previous history cache: %+v", row)
	}
	for _, s := range a.GetSyncState() {
		if s.DatabaseKey == "habits" && (s.HistoryOK == nil || *s.HistoryOK || s.HistoryErrorKind != dto.ErrorKindServer) {
			t.Fatalf("habit sync state = %+v, want history failure", s)
		}
	}
	if got := a.TraySummary(); got.HasError {
		t.Fatalf("history failure must not turn the label into '!': %+v", got)
	}
}

func TestHistory_DiscardsResultFromOldGeneration(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	a := newWeeklyApp(t, "http://127.0.0.1:0", &clock)
	gen := a.currentGeneration()
	a.invalidateCaches()
	if a.storeHistory("habits", gen, []notion.HabitPage{{Title: "stale"}}, "2026-11-12", nil) {
		t.Fatalf("storeHistory must refuse an old generation")
	}
	a.cacheMu.Lock()
	_, ok := a.historyCache["habits"]
	a.cacheMu.Unlock()
	if ok {
		t.Fatalf("stale history must not be written")
	}
}

func TestRequestBudget(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	ctx := context.Background()

	// 起動直後の 1 分: ポーリング 1 回＋過去分の取得。
	if err := a.refreshAll(ctx); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	task, week, hist := fake.counts()
	if total := task + week + hist; total > 60 || hist != 1 {
		t.Fatalf("first minute requests = %d (task %d, week %d, hist %d)", total, task, week, hist)
	}
	// 平常時: 60 秒ごとのポーリングでは、DB ごとに 1 回ずつ（過去分は取らない）。
	if err := a.refreshAll(ctx); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	task2, week2, hist2 := fake.counts()
	if per := (task2 - task) + (week2 - week) + (hist2 - hist); per != 2 {
		t.Fatalf("steady poll requests = %d, want 2", per)
	}
}

func TestHistory_StaleRunDoesNotAdvanceRetry(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	a := newWeeklyApp(t, "http://127.0.0.1:0", &clock)
	a.historyMu.Lock()
	a.history.running = true
	a.historyMu.Unlock()
	a.finishHistoryFetch("2026-11-12", false, true)
	a.historyMu.Lock()
	defer a.historyMu.Unlock()
	if !a.history.nextRetryAt.IsZero() || a.history.retryStep != 0 || a.history.fetchedFor != "" || a.history.running {
		t.Fatalf("stale run must not change retry state: %+v", a.history)
	}
}

func TestHistory_FailureWithoutCacheShowsDashNotLoading(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	fake := standardHabitFixture()
	fake.failHistory.Store(true)
	a := newWeeklyApp(t, fake.start(t).URL, &clock)
	if err := a.refreshAll(context.Background()); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	summary, err := a.GetHabitSummary("habits")
	if err != nil {
		t.Fatalf("GetHabitSummary: %v", err)
	}
	if summary.HistoryLoading {
		t.Fatalf("history_loading must be false after a failure: %+v", summary)
	}
	if row := rowByName(t, summary, "ストレッチ"); row.Streak != nil {
		t.Fatalf("streak must be nil (shown as —) when history never loaded: %+v", row)
	}
}

func TestHistory_TruncatedIsTreatedAsFailure(t *testing.T) {
	t.Parallel()

	clock := summaryNow
	a := newWeeklyApp(t, "http://127.0.0.1:0", &clock)
	gen := a.currentGeneration()
	if !a.storeHistory("habits", gen, nil, "2026-11-12", errHistoryTruncated) {
		t.Fatalf("storeHistory must accept the current generation")
	}
	a.cacheMu.Lock()
	rec := a.historyCache["habits"]
	a.cacheMu.Unlock()
	if rec.ok || rec.loaded || !rec.attempted {
		t.Fatalf("truncated history must be a failure that keeps no partial data: %+v", rec)
	}
}
