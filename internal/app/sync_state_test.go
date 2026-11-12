package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "401", err: &notion.APIError{Status: 401}, want: dto.ErrorKindAuth},
		{name: "403", err: &notion.APIError{Status: 403}, want: dto.ErrorKindAuth},
		{name: "404", err: &notion.APIError{Status: 404}, want: dto.ErrorKindNotShared},
		{name: "429", err: &notion.APIError{Status: 429}, want: dto.ErrorKindRateLimited},
		{name: "500", err: &notion.APIError{Status: 500}, want: dto.ErrorKindServer},
		{name: "400", err: &notion.APIError{Status: 400}, want: dto.ErrorKindUnknown},
		{name: "timeout", err: fmt.Errorf("http do: %w", timeoutErr{}), want: dto.ErrorKindNetwork},
		{name: "deadline", err: context.DeadlineExceeded, want: dto.ErrorKindNetwork},
		{name: "config", err: dto.DatabaseConfig{}.ValidateForHabit(""), want: dto.ErrorKindConfig},
		{name: "other", err: errors.New("boom"), want: dto.ErrorKindUnknown},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := classifyError(tt.err); got != tt.want {
				t.Fatalf("classifyError = %q, want %q", got, tt.want)
			}
		})
	}
}

// switchableServer はタスクの問い合わせに成功・失敗を切り替えて応える。
type switchableServer struct {
	fail     atomic.Bool
	mu       sync.Mutex
	bodies   []map[string]any
	response string
}

func (s *switchableServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		s.mu.Unlock()
		if s.fail.Load() {
			http.Error(w, `{"code":"object_not_found"}`, http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(s.response))
	}))
	t.Cleanup(srv.Close)
	return srv
}

const oneTaskResponse = `{"results":[{"id":"t1","url":"u","last_edited_time":"2026-11-10T00:00:00Z","properties":{
	"Name":{"type":"title","title":[{"plain_text":"タスク1"}]},
	"Status":{"type":"status","status":{"name":"進行中"}}}}],"has_more":false}`

func newSyncTestApp(t *testing.T, url string, db dto.DatabaseConfig) *App {
	t.Helper()
	cfg := dto.Config{Databases: []dto.DatabaseConfig{db}, MaxResults: 30}
	client := notion.NewClient(&stubTokenStore{token: "test"}, notion.WithBaseURL(url), notion.WithRetry(0, 0))
	a := NewApp(&stubConfigStore{cfg: cfg}, &stubTokenStore{token: "test"}, client)
	if _, err := a.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	a.now = func() time.Time { return time.Date(2026, 11, 12, 9, 0, 0, 0, jst) }
	a.loc = jst
	return a
}

func legacyTaskDB() dto.DatabaseConfig {
	return dto.DatabaseConfig{
		Key:                "tasks",
		Name:               "タスク",
		Kind:               dto.DatabaseKindTask,
		Enabled:            true,
		DatabaseID:         "db-1",
		DataSourceID:       "ds-1",
		TitlePropertyName:  "Name",
		StatusPropertyName: "Status",
		StatusPropertyType: "status",
		StatusInProgress:   "進行中",
		StatusDone:         "完了",
		StatusPaused:       "中断",
	}
}

func TestGetTasks_FailureKeepsCache(t *testing.T) {
	t.Parallel()

	srv := &switchableServer{response: oneTaskResponse}
	a := newSyncTestApp(t, srv.start(t).URL, legacyTaskDB())
	ctx := context.Background()

	if _, err := a.GetTasks(ctx, "tasks", true); err != nil {
		t.Fatalf("first GetTasks: %v", err)
	}
	srv.fail.Store(true)
	tasks, err := a.GetTasks(ctx, "tasks", true)
	if err != nil {
		t.Fatalf("GetTasks with cache must not fail: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Fatalf("tasks = %+v, want cached t1", tasks)
	}
	states := a.GetSyncState()
	if len(states) != 1 || states[0].OK || states[0].ErrorKind != dto.ErrorKindNotShared {
		t.Fatalf("sync state = %+v, want failed not_shared", states)
	}
	if states[0].LastSuccessAt == "" || states[0].LastAttemptAt == "" {
		t.Fatalf("sync state times must be set: %+v", states[0])
	}
}

func TestGetTasks_FailureWithoutCacheReturnsError(t *testing.T) {
	t.Parallel()

	srv := &switchableServer{response: oneTaskResponse}
	srv.fail.Store(true)
	a := newSyncTestApp(t, srv.start(t).URL, legacyTaskDB())

	if _, err := a.GetTasks(context.Background(), "tasks", true); err == nil {
		t.Fatalf("err = nil, want error without cache")
	}
}

func TestGetSyncState_NotAttemptedIsOK(t *testing.T) {
	t.Parallel()

	a := newSyncTestApp(t, "http://127.0.0.1:0", legacyTaskDB())
	states := a.GetSyncState()
	if len(states) != 1 || !states[0].OK || states[0].LastAttemptAt != "" || states[0].HistoryOK != nil {
		t.Fatalf("sync state = %+v, want untouched", states)
	}
}

func TestStoreResult_DiscardsOldGeneration(t *testing.T) {
	t.Parallel()

	a := newSyncTestApp(t, "http://127.0.0.1:0", legacyTaskDB())
	gen := a.currentGeneration()
	a.invalidateCaches()
	a.storeResult(dto.DatabaseKindTask, "tasks", gen, []dto.Task{{ID: "stale"}}, false, nil)

	if _, ok := a.getTaskCache("tasks"); ok {
		t.Fatalf("stale result must not be written after settings change")
	}
	if states := a.GetSyncState(); states[0].LastAttemptAt != "" {
		t.Fatalf("stale sync state must not be written: %+v", states[0])
	}
}

func TestV1CompatTaskQuery(t *testing.T) {
	t.Parallel()

	srv := &switchableServer{response: oneTaskResponse}
	a := newSyncTestApp(t, srv.start(t).URL, legacyTaskDB())

	tasks, err := a.GetTasks(context.Background(), "tasks", true)
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	filter, _ := srv.bodies[0]["filter"].(map[string]any)
	if _, isCompound := filter["or"]; isCompound {
		t.Fatalf("v1 config must use the in-progress-only filter: %v", filter)
	}
	if srv.bodies[0]["page_size"] != float64(30) {
		t.Fatalf("page_size = %v, want max_results 30", srv.bodies[0]["page_size"])
	}
	if len(tasks) != 1 || tasks[0].Group != TaskGroupInProgress || !tasks[0].IsInProgress {
		t.Fatalf("tasks = %+v", tasks)
	}
}

func TestSaveConfig_InvalidatesCaches(t *testing.T) {
	t.Parallel()

	srv := &switchableServer{response: oneTaskResponse}
	a := newSyncTestApp(t, srv.start(t).URL, legacyTaskDB())
	if _, err := a.GetTasks(context.Background(), "tasks", true); err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if err := a.SaveConfig(a.GetConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if _, ok := a.getTaskCache("tasks"); ok {
		t.Fatalf("cache must be cleared after SaveConfig")
	}
	if states := a.GetSyncState(); states[0].LastAttemptAt != "" {
		t.Fatalf("sync state must be cleared after SaveConfig: %+v", states[0])
	}
}

func TestGetTasks_RetriesOnceWhenSettingsChangeDuringRefresh(t *testing.T) {
	t.Parallel()

	var (
		a        *App
		requests atomic.Int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1 回目の問い合わせの途中で設定が保存された状況を作る。
		if requests.Add(1) == 1 {
			if err := a.SaveConfig(a.GetConfig()); err != nil {
				t.Errorf("SaveConfig: %v", err)
			}
		}
		_, _ = w.Write([]byte(oneTaskResponse))
	}))
	t.Cleanup(srv.Close)
	a = newSyncTestApp(t, srv.URL, legacyTaskDB())

	tasks, err := a.GetTasks(context.Background(), "tasks", true)
	if err != nil {
		t.Fatalf("GetTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "t1" {
		t.Fatalf("tasks = %+v, want t1 after one retry (not an empty success)", tasks)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2 (retry once)", got)
	}
}

func TestClassifyError_AfterRetriesExhausted(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		http.Error(w, `{"code":"rate_limited"}`, http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	client := notion.NewClient(&stubTokenStore{token: "test"}, notion.WithBaseURL(srv.URL), notion.WithRetry(1, 0))
	a := NewApp(&stubConfigStore{cfg: dto.Config{Databases: []dto.DatabaseConfig{legacyTaskDB()}}}, &stubTokenStore{token: "test"}, client)
	if _, err := a.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	_, err := a.GetTasks(context.Background(), "tasks", true)
	if got := classifyError(err); got != dto.ErrorKindRateLimited {
		t.Fatalf("classifyError = %q (err=%v), want rate_limited", got, err)
	}
	if states := a.GetSyncState(); states[0].ErrorKind != dto.ErrorKindRateLimited {
		t.Fatalf("sync state = %+v", states[0])
	}
}
