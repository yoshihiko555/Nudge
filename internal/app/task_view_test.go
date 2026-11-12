package app

import (
	"reflect"
	"testing"
	"time"

	"nudge/internal/dto"
)

var jst = time.FixedZone("JST", 9*60*60)

func dueDB() dto.DatabaseConfig {
	return dto.DatabaseConfig{
		Key:                  "tasks",
		Kind:                 dto.DatabaseKindTask,
		StatusPropertyName:   "ステータス",
		StatusPropertyType:   "status",
		StatusInProgress:     "進行中",
		StatusDone:           "完了",
		StatusPaused:         "中断",
		DueDatePropertyName:  "期限日",
		PriorityPropertyName: "優先度",
		PriorityOrder:        []string{"高", "中", "低"},
	}
}

func task(id, status, due, priority, edited string) dto.Task {
	return dto.Task{ID: id, Status: status, DueDate: due, Priority: priority, LastEditedTime: edited}
}

func groupsOf(view []dto.Task) map[string]string {
	out := make(map[string]string, len(view))
	for _, t := range view {
		out[t.ID] = t.Group
	}
	return out
}

func idsOf(view []dto.Task) []string {
	out := make([]string, 0, len(view))
	for _, t := range view {
		out = append(out, t.ID)
	}
	return out
}

// 今日は 2026-11-12（木）。
var taskViewNow = time.Date(2026, 11, 12, 0, 30, 0, 0, jst)

func TestBuildTaskView_GroupBoundaries(t *testing.T) {
	t.Parallel()

	tasks := []dto.Task{
		task("yesterday-waiting", "待ち", "2026-11-11", "", ""),
		task("today-hold", "保留", "2026-11-12", "", ""),
		task("tomorrow-review", "確認中", "2026-11-13", "", ""),
		task("plus7-waiting", "待ち", "2026-11-19", "", ""),
		task("plus8-waiting", "待ち", "2026-11-20", "", ""),
		task("progress-no-due", "進行中", "", "", ""),
		task("progress-tomorrow", "進行中", "2026-11-13", "", ""),
		task("progress-overdue", "進行中", "2026-11-01", "", ""),
		task("waiting-no-due", "待ち", "", "", ""),
	}
	got := groupsOf(buildTaskView(tasks, dueDB(), taskViewNow))
	want := map[string]string{
		"yesterday-waiting": TaskGroupOverdue,
		"today-hold":        TaskGroupToday,
		"tomorrow-review":   TaskGroupUpcoming,
		"plus7-waiting":     TaskGroupUpcoming,
		"progress-no-due":   TaskGroupInProgress,
		"progress-tomorrow": TaskGroupInProgress,
		"progress-overdue":  TaskGroupOverdue,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildTaskView_SetsIsInProgress(t *testing.T) {
	t.Parallel()

	view := buildTaskView([]dto.Task{
		task("a", "進行中", "2026-11-12", "", ""),
		task("b", "待ち", "2026-11-12", "", ""),
	}, dueDB(), taskViewNow)
	for _, tk := range view {
		if (tk.ID == "a") != tk.IsInProgress {
			t.Fatalf("task %s IsInProgress = %v", tk.ID, tk.IsInProgress)
		}
	}
}

func TestBuildTaskView_SortOrder(t *testing.T) {
	t.Parallel()

	tasks := []dto.Task{
		task("up-late", "待ち", "2026-11-18", "高", "2026-11-01T00:00:00Z"),
		task("up-early", "待ち", "2026-11-14", "低", "2026-11-01T00:00:00Z"),
		task("prog-low-new", "進行中", "", "低", "2026-11-11T00:00:00Z"),
		task("prog-high-old", "進行中", "2026-11-30", "高", "2026-11-01T00:00:00Z"),
		task("today-mid", "待ち", "2026-11-12", "中", "2026-11-01T00:00:00Z"),
		task("today-high-old", "待ち", "2026-11-12", "高", "2026-11-01T00:00:00Z"),
		task("today-high-new", "待ち", "2026-11-12", "高", "2026-11-10T00:00:00Z"),
		task("over-new-date", "待ち", "2026-11-10", "高", "2026-11-01T00:00:00Z"),
		task("over-old-date", "待ち", "2026-11-01", "低", "2026-11-01T00:00:00Z"),
	}
	got := idsOf(buildTaskView(tasks, dueDB(), taskViewNow))
	want := []string{
		"over-old-date", "over-new-date", // 期限切れ: 期日の古い順
		"today-high-new", "today-high-old", "today-mid", // 今日: 優先度 → 最終編集の新しい順
		"prog-high-old", "prog-low-new", // 進行中: 優先度（期日では並べない）
		"up-early", "up-late", // 直近 7 日: 期日の近い順
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildTaskView_PriorityNotInOrderGoesLast(t *testing.T) {
	t.Parallel()

	tasks := []dto.Task{
		task("empty", "進行中", "", "", "2026-11-12T00:00:00Z"),
		task("unknown", "進行中", "", "緊急", "2026-11-11T00:00:00Z"),
		task("low", "進行中", "", "低", "2026-11-01T00:00:00Z"),
	}
	got := idsOf(buildTaskView(tasks, dueDB(), taskViewNow))
	want := []string{"low", "empty", "unknown"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestBuildTaskView_EmptyPriorityOrderSortsByEdited(t *testing.T) {
	t.Parallel()

	db := dueDB()
	db.PriorityOrder = nil
	tasks := []dto.Task{
		task("old-high", "進行中", "", "高", "2026-11-01T00:00:00Z"),
		task("new-low", "進行中", "", "低", "2026-11-11T00:00:00Z"),
	}
	got := idsOf(buildTaskView(tasks, db, taskViewNow))
	if !reflect.DeepEqual(got, []string{"new-low", "old-high"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestBuildTaskView_V1CompatKeepsOrderAndInProgress(t *testing.T) {
	t.Parallel()

	db := dueDB()
	db.DueDatePropertyName = ""
	tasks := []dto.Task{
		task("first", "進行中", "", "低", "2026-11-11T00:00:00Z"),
		task("second", "進行中", "", "高", "2026-11-01T00:00:00Z"),
	}
	view := buildTaskView(tasks, db, taskViewNow)
	if !reflect.DeepEqual(idsOf(view), []string{"first", "second"}) {
		t.Fatalf("order = %v, want v1 order", idsOf(view))
	}
	for _, tk := range view {
		if tk.Group != TaskGroupInProgress || !tk.IsInProgress {
			t.Fatalf("task %s group=%s inProgress=%v", tk.ID, tk.Group, tk.IsInProgress)
		}
	}
}

func TestBuildTaskView_RecomputesAfterMidnight(t *testing.T) {
	t.Parallel()

	tasks := []dto.Task{task("a", "待ち", "2026-11-12", "", "")}
	before := buildTaskView(tasks, dueDB(), time.Date(2026, 11, 12, 23, 59, 0, 0, jst))
	after := buildTaskView(tasks, dueDB(), time.Date(2026, 11, 13, 0, 0, 0, 0, jst))
	if before[0].Group != TaskGroupToday || after[0].Group != TaskGroupOverdue {
		t.Fatalf("before=%s after=%s, want today → overdue", before[0].Group, after[0].Group)
	}
}

func TestCountDueTasks(t *testing.T) {
	t.Parallel()

	view := []dto.Task{
		{Group: TaskGroupOverdue}, {Group: TaskGroupToday}, {Group: TaskGroupInProgress}, {Group: TaskGroupUpcoming},
	}
	if got := countDueTasks(view); got != 2 {
		t.Fatalf("countDueTasks = %d, want 2", got)
	}
}
