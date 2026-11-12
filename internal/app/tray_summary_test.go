package app

import (
	"context"
	"testing"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

func TestFormatTrayLabel(t *testing.T) {
	t.Parallel()

	ready := TraySummary{Active: true, Ready: true}
	tests := []struct {
		name string
		in   TraySummary
		want string
	}{
		{name: "トークンや有効な DB が無い", in: TraySummary{Active: false, Ready: true, DueCount: 3}, want: ""},
		{name: "最初の取得の前", in: TraySummary{Active: true, Ready: false, DueCount: 3}, want: ""},
		{name: "取得に失敗", in: TraySummary{Active: true, Ready: true, HasError: true, DueCount: 3, HabitCount: 2}, want: "!"},
		{name: "両方ある", in: TraySummary{Active: true, Ready: true, DueCount: 3, HabitCount: 2}, want: "⚠3 ✓2"},
		{name: "タスクだけ", in: TraySummary{Active: true, Ready: true, DueCount: 1}, want: "⚠1"},
		{name: "習慣だけ", in: TraySummary{Active: true, Ready: true, HabitCount: 4}, want: "✓4"},
		{name: "両方 0", in: ready, want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FormatTrayLabel(tt.in); got != tt.want {
				t.Fatalf("FormatTrayLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func trayTestApp(t *testing.T) *App {
	t.Helper()
	taskDB := legacyTaskDB()
	taskDB.DueDatePropertyName = "期限日"
	habitDB := dto.DatabaseConfig{Key: "habits", Name: "習慣", Kind: dto.DatabaseKindHabit, Enabled: true, CheckboxPropertyName: "日,月,火,水,木,金,土"}
	disabled := legacyTaskDB()
	disabled.Key = "off"
	disabled.Enabled = false
	cfg := dto.Config{Databases: []dto.DatabaseConfig{taskDB, habitDB, disabled}}
	a := NewApp(&stubConfigStore{cfg: cfg}, &stubTokenStore{token: "test"}, nil)
	if _, err := a.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	a.now = func() time.Time { return time.Date(2026, 11, 12, 9, 0, 0, 0, jst) }
	a.loc = jst
	return a
}

func TestTraySummary_CountsDueTasksAndHabits(t *testing.T) {
	t.Parallel()

	a := trayTestApp(t)
	a.cacheMu.Lock()
	a.ready = true
	a.taskCache["tasks"] = []dto.Task{
		{ID: "over", Status: "待ち", DueDate: "2026-11-10"},
		{ID: "today", Status: "進行中", DueDate: "2026-11-12"},
		{ID: "later", Status: "待ち", DueDate: "2026-11-14"},
		{ID: "progress", Status: "進行中"},
	}
	// 今日は木曜（today_index 4）。残っている習慣は、今日が未チェックで今週の目標に届いていないものだけ。
	created := time.Date(2026, 11, 8, 0, 30, 0, 0, jst)
	a.habitWeek["habits"] = habitWeekRecord{loaded: true, pages: []notion.HabitPage{
		{ID: "remain-1", Title: "ストレッチ", CreatedTime: created, Goal: "週３回"},
		{ID: "remain-2", Title: "読書", CreatedTime: created, Goal: ""},
		{ID: "checked-today", Title: "掃除", CreatedTime: created, Goal: "週２回", Days: [7]bool{4: true}},
		{ID: "goal-reached", Title: "薬", CreatedTime: created, Goal: "週２回", Days: [7]bool{true, true}},
	}}
	a.taskCache["off"] = []dto.Task{{ID: "ignored", Status: "待ち", DueDate: "2026-11-01"}}
	a.cacheMu.Unlock()

	got := a.TraySummary()
	if !got.Active || !got.Ready || got.HasError || got.DueCount != 2 || got.HabitCount != 2 {
		t.Fatalf("summary = %+v, want active ready due=2 habit=2", got)
	}
	if label := FormatTrayLabel(got); label != "⚠2 ✓2" {
		t.Fatalf("label = %q", label)
	}
}

func TestTraySummary_ErrorFromAnyEnabledDatabase(t *testing.T) {
	t.Parallel()

	a := trayTestApp(t)
	a.cacheMu.Lock()
	a.ready = true
	a.syncStates["habits"] = syncRecord{attempted: true, ok: false, errorKind: dto.ErrorKindNetwork}
	a.syncStates["off"] = syncRecord{attempted: true, ok: false}
	a.cacheMu.Unlock()

	if got := a.TraySummary(); !got.HasError {
		t.Fatalf("summary = %+v, want HasError", got)
	}
}

func TestTraySummary_InactiveWithoutToken(t *testing.T) {
	t.Parallel()

	a := trayTestApp(t)
	a.tokenStore = &stubTokenStore{token: ""}
	if got := a.TraySummary(); got.Active {
		t.Fatalf("summary = %+v, want inactive without token", got)
	}
}

func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("no summary signal")
	}
}

func TestNotifier_SignalsAfterRefreshAndForcedFetch(t *testing.T) {
	t.Parallel()

	srv := &switchableServer{response: oneTaskResponse}
	a := newSyncTestApp(t, srv.start(t).URL, legacyTaskDB())
	ch := make(chan struct{}, 1)
	a.SetSummaryNotifier(ch)

	if err := a.refreshAll(context.Background()); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	waitSignal(t, ch)
	if got := a.TraySummary(); !got.Ready {
		t.Fatalf("summary = %+v, want ready after first refresh", got)
	}

	if _, err := a.GetTasks(context.Background(), "tasks", true); err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	waitSignal(t, ch)
}

func TestNotifier_DoesNotBlockWhenChannelIsFull(t *testing.T) {
	t.Parallel()

	a := trayTestApp(t)
	ch := make(chan struct{}, 1)
	a.SetSummaryNotifier(ch)
	done := make(chan struct{})
	go func() {
		a.notifySummary()
		a.notifySummary()
		a.notifySummary()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("notifySummary blocked on a full channel")
	}
	if len(ch) != 1 {
		t.Fatalf("pending signals = %d, want 1", len(ch))
	}
}

func TestSaveConfig_KeepsLabelUntilReloadFinishes(t *testing.T) {
	t.Parallel()

	srv := &switchableServer{response: oneTaskResponse}
	a := newSyncTestApp(t, srv.start(t).URL, legacyTaskDB())
	if err := a.refreshAll(context.Background()); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	if err := a.SaveConfig(a.GetConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if got := a.TraySummary(); !got.Reloading || !got.Ready {
		t.Fatalf("summary = %+v, want reloading and still ready after save", got)
	}
	if err := a.refreshAll(context.Background()); err != nil {
		t.Fatalf("refreshAll: %v", err)
	}
	if got := a.TraySummary(); got.Reloading {
		t.Fatalf("summary = %+v, want reload finished", got)
	}
}
