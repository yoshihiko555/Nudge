package notion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"nudge/internal/dto"
)

type stubTokenStore struct{}

func (stubTokenStore) GetToken() (string, error) { return "test-token", nil }
func (stubTokenStore) SetToken(string) error     { return nil }
func (stubTokenStore) ClearToken() error         { return nil }

var jst = time.FixedZone("JST", 9*60*60)

func dueTaskDB(statusType string) dto.DatabaseConfig {
	return dto.DatabaseConfig{
		Key:                  "tasks",
		Kind:                 dto.DatabaseKindTask,
		DataSourceID:         "ds-1",
		TitlePropertyName:    "タスク名",
		StatusPropertyName:   "ステータス",
		StatusPropertyType:   statusType,
		StatusInProgress:     "進行中",
		StatusDone:           "完了",
		StatusPaused:         "中断",
		DueDatePropertyName:  "期限日",
		PriorityPropertyName: "優先度",
	}
}

// toJSONMap は比較のために map を JSON 経由で正規化する。
func toJSONMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

func TestBuildDueTaskFilter(t *testing.T) {
	t.Parallel()

	for _, typ := range []string{"status", "select"} {
		typ := typ
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			got := toJSONMap(t, buildDueTaskFilter(dueTaskDB(typ), "2026-11-20"))
			want := toJSONMap(t, map[string]any{
				"or": []any{
					map[string]any{"property": "ステータス", typ: map[string]any{"equals": "進行中"}},
					map[string]any{"and": []any{
						map[string]any{"property": "期限日", "date": map[string]any{"on_or_before": "2026-11-20"}},
						map[string]any{"property": "ステータス", typ: map[string]any{"does_not_equal": "完了"}},
						map[string]any{"property": "ステータス", typ: map[string]any{"does_not_equal": "中断"}},
					}},
				},
			})
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("filter mismatch\n got: %v\nwant: %v", got, want)
			}
		})
	}
}

type queryServer struct {
	mu       sync.Mutex
	bodies   []map[string]any
	handlers []func(w http.ResponseWriter)
}

func (q *queryServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		q.mu.Lock()
		idx := len(q.bodies)
		q.bodies = append(q.bodies, body)
		q.mu.Unlock()
		if idx >= len(q.handlers) {
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		q.handlers[idx](w)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writePage(cursor string, hasMore bool, ids ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		results := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			results = append(results, map[string]any{"id": id, "properties": map[string]any{}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results":     results,
			"has_more":    hasMore,
			"next_cursor": cursor,
		})
	}
}

func newTestClient(url string) *Client {
	return NewClient(stubTokenStore{}, WithBaseURL(url), WithRetry(0, 0))
}

func TestQueryAll_PaginatesWithCursor(t *testing.T) {
	t.Parallel()

	q := &queryServer{handlers: []func(http.ResponseWriter){
		writePage("cursor-2", true, "a", "b"),
		writePage("", false, "c"),
	}}
	c := newTestClient(q.start(t).URL)

	pages, truncated, err := c.queryAll(context.Background(), "ds-1", map[string]any{"sorts": []any{}}, "")
	if err != nil {
		t.Fatalf("queryAll: %v", err)
	}
	if truncated {
		t.Fatalf("truncated = true, want false")
	}
	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(pages))
	}
	if len(q.bodies) != 2 {
		t.Fatalf("requests = %d, want 2", len(q.bodies))
	}
	if _, ok := q.bodies[0]["start_cursor"]; ok {
		t.Fatalf("first request must not have start_cursor")
	}
	if q.bodies[1]["start_cursor"] != "cursor-2" {
		t.Fatalf("second start_cursor = %v, want cursor-2", q.bodies[1]["start_cursor"])
	}
	for i, body := range q.bodies {
		if body["page_size"] != float64(queryPageSize) {
			t.Fatalf("request %d page_size = %v, want %d", i, body["page_size"], queryPageSize)
		}
	}
}

func TestQueryAll_PaginationStopsWhenHasMoreFalse(t *testing.T) {
	t.Parallel()

	ids := make([]string, queryPageSize)
	for i := range ids {
		ids[i] = fmt.Sprintf("p-%d", i)
	}
	q := &queryServer{handlers: []func(http.ResponseWriter){writePage("", false, ids...)}}
	c := newTestClient(q.start(t).URL)

	pages, truncated, err := c.queryAll(context.Background(), "ds-1", map[string]any{}, "")
	if err != nil {
		t.Fatalf("queryAll: %v", err)
	}
	if len(pages) != queryPageSize || truncated || len(q.bodies) != 1 {
		t.Fatalf("pages=%d truncated=%v requests=%d, want %d false 1", len(pages), truncated, len(q.bodies), queryPageSize)
	}
}

func TestQueryAll_PaginationFailsMidway(t *testing.T) {
	t.Parallel()

	q := &queryServer{handlers: []func(http.ResponseWriter){
		writePage("cursor-2", true, "a"),
		func(w http.ResponseWriter) { http.Error(w, `{"code":"validation_error"}`, http.StatusBadRequest) },
	}}
	c := newTestClient(q.start(t).URL)

	pages, _, err := c.queryAll(context.Background(), "ds-1", map[string]any{}, "")
	if err == nil {
		t.Fatalf("err = nil, want error")
	}
	if pages != nil {
		t.Fatalf("pages = %v, want nil on failure", pages)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest || apiErr.Code != "validation_error" {
		t.Fatalf("err = %#v, want APIError 400 validation_error", err)
	}
}

func TestQueryAll_PaginationTruncatesAtMaxPages(t *testing.T) {
	t.Parallel()

	handlers := make([]func(http.ResponseWriter), maxQueryPages)
	for i := range handlers {
		handlers[i] = writePage(fmt.Sprintf("cursor-%d", i+1), true, fmt.Sprintf("p-%d", i))
	}
	q := &queryServer{handlers: handlers}
	c := newTestClient(q.start(t).URL)

	pages, truncated, err := c.queryAll(context.Background(), "ds-1", map[string]any{}, "")
	if err != nil {
		t.Fatalf("queryAll: %v", err)
	}
	if !truncated || len(pages) != maxQueryPages || len(q.bodies) != maxQueryPages {
		t.Fatalf("truncated=%v pages=%d requests=%d, want true %d %d", truncated, len(pages), len(q.bodies), maxQueryPages, maxQueryPages)
	}
}

func TestQueryTasks_FilterUsesLookahead(t *testing.T) {
	t.Parallel()

	q := &queryServer{handlers: []func(http.ResponseWriter){func(w http.ResponseWriter) {
		_, _ = w.Write([]byte(`{"results":[{"id":"t1","url":"u","last_edited_time":"2026-11-10T00:00:00Z","properties":{
			"タスク名":{"type":"title","title":[{"plain_text":"請求書"}]},
			"ステータス":{"type":"status","status":{"name":"待ち"}},
			"期限日":{"type":"date","date":{"start":"2026-11-10","end":null,"time_zone":null}},
			"優先度":{"type":"select","select":{"name":"高"}}}}],"has_more":false}`))
	}}}
	c := newTestClient(q.start(t).URL)
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, jst)

	res, err := c.QueryTasks(context.Background(), dueTaskDB("status"), "", 30, now)
	if err != nil {
		t.Fatalf("QueryTasks: %v", err)
	}
	filter := q.bodies[0]["filter"].(map[string]any)
	and := filter["or"].([]any)[1].(map[string]any)["and"].([]any)
	date := and[0].(map[string]any)["date"].(map[string]any)
	if date["on_or_before"] != "2026-11-20" {
		t.Fatalf("on_or_before = %v, want 2026-11-20", date["on_or_before"])
	}
	if len(res.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(res.Tasks))
	}
	got := res.Tasks[0]
	if got.Title != "請求書" || got.Status != "待ち" || got.DueDate != "2026-11-10" || got.Priority != "高" {
		t.Fatalf("task = %+v", got)
	}
}

func TestQueryTasks_LegacyFilterWithoutDueDate(t *testing.T) {
	t.Parallel()

	q := &queryServer{handlers: []func(http.ResponseWriter){writePage("", false)}}
	c := newTestClient(q.start(t).URL)
	db := dueTaskDB("status")
	db.DueDatePropertyName = ""

	if _, err := c.QueryTasks(context.Background(), db, "", 30, time.Now()); err != nil {
		t.Fatalf("QueryTasks: %v", err)
	}
	got := q.bodies[0]
	want := toJSONMap(t, map[string]any{
		"filter":    map[string]any{"property": "ステータス", "status": map[string]any{"equals": "進行中"}},
		"sorts":     []any{map[string]any{"timestamp": "last_edited_time", "direction": "descending"}},
		"page_size": 30,
	})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy body mismatch\n got: %v\nwant: %v", got, want)
	}
}

func TestParseNotionDueDate(t *testing.T) {
	t.Parallel()

	end := "2026-11-15"
	tokyo := "Asia/Tokyo"
	tests := []struct {
		name string
		in   *dateValue
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "date only", in: &dateValue{Start: "2026-11-12"}, want: "2026-11-12"},
		{name: "offset jst", in: &dateValue{Start: "2026-11-12T23:30:00.000+09:00"}, want: "2026-11-12"},
		{name: "utc becomes next day in jst", in: &dateValue{Start: "2026-11-12T15:30:00.000Z"}, want: "2026-11-13"},
		{name: "range uses end", in: &dateValue{Start: "2026-11-10", End: &end}, want: "2026-11-15"},
		{name: "no offset with time_zone", in: &dateValue{Start: "2026-11-12T23:30:00.000", TimeZone: &tokyo}, want: "2026-11-12"},
		{name: "no offset without time_zone uses local", in: &dateValue{Start: "2026-11-12T23:30:00.000"}, want: "2026-11-12"},
		{name: "invalid", in: &dateValue{Start: "11/12"}, want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := dueDateOf(tt.in, jst); got != tt.want {
				t.Fatalf("dueDateOf = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_KeepsV1MessageAndParsesCode(t *testing.T) {
	t.Parallel()

	body := []byte(`{"object":"error","status":404,"code":"object_not_found","message":"not shared"}`)
	e := newAPIError(http.StatusNotFound, body)
	if e.Status != 404 || e.Code != "object_not_found" || e.Message != "not shared" {
		t.Fatalf("APIError = %+v", e)
	}
	want := "notion error: status=404 body=" + string(body)
	if e.Error() != want {
		t.Fatalf("Error() = %q, want %q", e.Error(), want)
	}
}
