package notion

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"nudge/internal/dto"
)

func TestBuildCreatedTimeFilter(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, 11, 8, 0, 0, 0, 0, jst)
	got := toJSONMap(t, buildCreatedTimeFilter(from, time.Time{}))
	want := toJSONMap(t, map[string]any{
		"timestamp":    "created_time",
		"created_time": map[string]any{"on_or_after": "2026-11-08T00:00:00+09:00"},
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("this week filter\n got: %v\nwant: %v", got, want)
	}

	got = toJSONMap(t, buildCreatedTimeFilter(from.AddDate(0, 0, -364), from))
	want = toJSONMap(t, map[string]any{"and": []any{
		map[string]any{"timestamp": "created_time", "created_time": map[string]any{"on_or_after": "2025-11-09T00:00:00+09:00"}},
		map[string]any{"timestamp": "created_time", "created_time": map[string]any{"before": "2026-11-08T00:00:00+09:00"}},
	}})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history filter\n got: %v\nwant: %v", got, want)
	}
}

func TestQueryHabitPages_MapsDaysAndGoal(t *testing.T) {
	t.Parallel()

	q := &queryServer{handlers: []func(http.ResponseWriter){func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"results":[
			{"id":"p1","url":"u1","created_time":"2026-11-07T15:31:00.000Z","properties":{
				"名前":{"type":"title","title":[{"plain_text":" 掃除 "}]},
				"日":{"type":"checkbox","checkbox":true},"水":{"type":"checkbox","checkbox":true},
				"ゴール":{"type":"select","select":{"name":"週２回"}}}},
			{"id":"p2","url":"u2","created_time":"2026-11-07T15:00:00.000Z","properties":{
				"名前":{"type":"title","title":[{"plain_text":"学習"}]},
				"ゴール":{"type":"rich_text","rich_text":[{"plain_text":"週"},{"plain_text":"3回"}]}}},
			{"id":"bad","url":"u3","created_time":"not-a-time","properties":{}}
		],"has_more":false}`))
	}}}
	c := newTestClient(q.start(t).URL)
	db := dto.DatabaseConfig{DataSourceID: "ds", TitlePropertyName: "名前", GoalPropertyName: "ゴール"}
	weekdays := []string{"日", "月", "火", "水", "木", "金", "土"}

	pages, truncated, err := c.QueryHabitPages(context.Background(), db, "", weekdays, time.Date(2026, 11, 8, 0, 0, 0, 0, jst), time.Time{})
	if err != nil || truncated {
		t.Fatalf("QueryHabitPages: err=%v truncated=%v", err, truncated)
	}
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2 (invalid created_time skipped)", len(pages))
	}
	if pages[0].Title != "掃除" || pages[0].Goal != "週２回" || pages[0].Days != [WeekdayCount]bool{true, false, false, true} {
		t.Fatalf("page 1 = %+v", pages[0])
	}
	if pages[1].Goal != "週3回" {
		t.Fatalf("rich_text goal = %q", pages[1].Goal)
	}
	if !pages[0].CreatedTime.Equal(time.Date(2026, 11, 8, 0, 31, 0, 0, jst)) {
		t.Fatalf("created time = %v", pages[0].CreatedTime)
	}
}
