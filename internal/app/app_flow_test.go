package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

type stubConfigStore struct {
	cfg dto.Config
}

func (s *stubConfigStore) Load() (dto.Config, error) { return s.cfg, nil }
func (s *stubConfigStore) Save(dto.Config) error     { return nil }
func (s *stubConfigStore) Path() (string, error)     { return "/tmp/test-config.json", nil }

type stubTokenStore struct {
	token string
}

func (s *stubTokenStore) GetToken() (string, error) { return s.token, nil }
func (s *stubTokenStore) SetToken(token string) error {
	s.token = token
	return nil
}
func (s *stubTokenStore) ClearToken() error {
	s.token = ""
	return nil
}

type recordedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

func TestTaskAndHabitFlow(t *testing.T) {
	t.Parallel()

	var (
		reqMu    sync.Mutex
		recorded []recordedRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var body map[string]any
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, fmt.Sprintf("decode body: %v", err), http.StatusBadRequest)
				return
			}
		}

		reqMu.Lock()
		recorded = append(recorded, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   body,
		})
		reqMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/data_sources/task-ds/query":
			_, _ = w.Write([]byte(`{"results":[{"id":"task-1","url":"https://example.com/task-1","last_edited_time":"2026-02-28T00:00:00Z","properties":{"Name":{"type":"title","title":[{"plain_text":"タスク1"}]},"Status":{"type":"status","status":{"name":"進行中"}}}}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/data_sources/habit-ds/query":
			_, _ = w.Write([]byte(`{"results":[{"id":"habit-1","url":"https://example.com/habit-1","last_edited_time":"2026-02-28T00:00:00Z","properties":{"HabitName":{"type":"title","title":[{"plain_text":"習慣A"}]},"DoneToday":{"type":"checkbox","checkbox":false}}},{"id":"habit-2","url":"https://example.com/habit-2","last_edited_time":"2026-02-28T00:00:00Z","properties":{"HabitName":{"type":"title","title":[{"plain_text":"習慣A"}]},"DoneToday":{"type":"checkbox","checkbox":true}}}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/pages/task-1":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/v1/pages/habit-1":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	tokenStore := &stubTokenStore{token: "dummy-token"}
	cfgStore := &stubConfigStore{
		cfg: dto.Config{
			Databases: []dto.DatabaseConfig{
				{
					Key:                "tasks",
					Name:               "タスク",
					Kind:               dto.DatabaseKindTask,
					Enabled:            true,
					DataSourceID:       "task-ds",
					TitlePropertyName:  "Name",
					StatusPropertyName: "Status",
					StatusPropertyType: "status",
					StatusInProgress:   "進行中",
					StatusDone:         "完了",
					StatusPaused:       "保留",
				},
				{
					Key:                  "habits",
					Name:                 "習慣",
					Kind:                 dto.DatabaseKindHabit,
					Enabled:              true,
					DataSourceID:         "habit-ds",
					TitlePropertyName:    "HabitName",
					CheckboxPropertyName: "DoneToday",
				},
			},
			MaxResults: 10,
		},
	}

	notionClient := notion.NewClient(
		tokenStore,
		notion.WithBaseURL(server.URL),
		notion.WithRetry(0, 0),
	)
	app := NewApp(cfgStore, tokenStore, notionClient)

	if _, err := app.LoadConfig(); err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	ctx := context.Background()

	tasks, err := app.GetTasks(ctx, "tasks", true)
	if err != nil {
		t.Fatalf("GetTasks failed: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" || tasks[0].Title != "タスク1" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}

	if err := app.UpdateTaskStatus(ctx, "tasks", "task-1", "done"); err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	habits, err := app.GetHabits(ctx, "habits", true)
	if err != nil {
		t.Fatalf("GetHabits failed: %v", err)
	}
	if len(habits) != 1 || habits[0].ID != "habit-1" || habits[0].Checked {
		t.Fatalf("unexpected habits: %+v", habits)
	}

	if err := app.UpdateHabitCheck(ctx, "habits", "habit-1", true); err != nil {
		t.Fatalf("UpdateHabitCheck failed: %v", err)
	}

	assertRequestContains(t, recorded, http.MethodPost, "/v1/data_sources/task-ds/query", nil)
	assertRequestContains(t, recorded, http.MethodPost, "/v1/data_sources/habit-ds/query", nil)
	assertRequestContains(t, recorded, http.MethodPatch, "/v1/pages/task-1", map[string]any{
		"properties": map[string]any{
			"Status": map[string]any{
				"status": map[string]any{
					"name": "完了",
				},
			},
		},
	})
	assertRequestContains(t, recorded, http.MethodPatch, "/v1/pages/habit-1", map[string]any{
		"properties": map[string]any{
			"DoneToday": map[string]any{
				"checkbox": true,
			},
		},
	})
}

func assertRequestContains(t *testing.T, requests []recordedRequest, method, path string, expectedBody map[string]any) {
	t.Helper()
	for _, req := range requests {
		if req.Method != method || req.Path != path {
			continue
		}
		if expectedBody == nil {
			return
		}
		if !deepEqualJSONMap(req.Body, expectedBody) {
			t.Fatalf("unexpected request body for %s %s: got=%v want=%v", method, path, req.Body, expectedBody)
		}
		return
	}
	t.Fatalf("request not found: %s %s", method, path)
}

func deepEqualJSONMap(got, want map[string]any) bool {
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}

func TestResolveHabitCheckboxProperty_EmptyReturnsError(t *testing.T) {
	t.Parallel()

	// Arrange
	db := dto.DatabaseConfig{
		CheckboxPropertyName: "",
	}

	// Act
	_, err := resolveHabitCheckboxProperty(db, time.Now())

	// Assert
	if err == nil {
		t.Fatal("expected error when CheckboxPropertyName is empty, got nil")
	}
}

func TestResolveHabitCheckboxProperty_InsufficientEntries(t *testing.T) {
	t.Parallel()

	// 3エントリしかないのに weekday=5 (Friday) を要求
	db := dto.DatabaseConfig{
		CheckboxPropertyName: "Mon,Tue,Wed",
	}

	// Friday = 5, len(parts) = 3 → エラーになるべき
	friday := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	_, err := resolveHabitCheckboxProperty(db, friday)
	if err == nil {
		t.Fatal("expected error when weekday index exceeds entries, got nil")
	}
}

func TestHasConfiguredDatabase_EmptyDatabases(t *testing.T) {
	t.Parallel()

	// Arrange
	cfg := dto.Config{
		Databases: nil,
	}

	// Act
	result := hasConfiguredDatabase(cfg)

	// Assert
	if result {
		t.Fatal("expected hasConfiguredDatabase to return false for empty Databases, got true")
	}
}
