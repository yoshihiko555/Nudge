package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"nudge/internal/dto"
	"nudge/internal/store"
)

const (
	defaultBaseURL       = "https://api.notion.com"
	defaultNotionVersion = "2025-09-03"

	// queryPageSize は 1 回の問い合わせで取る件数（Notion の上限）。
	queryPageSize = 100
	// maxQueryPages はページ送りの打ち切り（5,000 件）。超えたら truncated として扱う。
	maxQueryPages = 50
	// dueDateLookaheadDays は問い合わせで期日を取る範囲（今日＋7 日に、UTC との差の余裕 1 日を足す）。
	dueDateLookaheadDays = 8
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	version    string
	tokenStore store.TokenStore
	maxRetries int
	retryWait  time.Duration
}

type Option func(*Client)

func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
}

func WithNotionVersion(version string) Option {
	return func(c *Client) { c.version = version }
}

func WithRetry(maxRetries int, wait time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.retryWait = wait
	}
}

func NewClient(tokenStore store.TokenStore, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    defaultBaseURL,
		version:    defaultNotionVersion,
		maxRetries: 3,
		retryWait:  2 * time.Second,
		tokenStore: tokenStore,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) QueryInProgress(ctx context.Context, db dto.DatabaseConfig, notionVersion string, maxResults int) ([]dto.Task, error) {
	return c.QueryByStatus(ctx, db, notionVersion, maxResults, db.StatusInProgress)
}

func (c *Client) QueryByStatus(ctx context.Context, db dto.DatabaseConfig, notionVersion string, maxResults int, statusValue string) ([]dto.Task, error) {
	if err := db.ValidateForTaskQuery(notionVersion, statusValue); err != nil {
		return nil, err
	}
	body := map[string]any{
		"filter": buildStatusFilter(db.StatusPropertyName, db.StatusPropertyType, statusValue),
		"sorts": []map[string]any{{
			"timestamp": "last_edited_time",
			"direction": "descending",
		}},
	}
	if maxResults > 0 {
		body["page_size"] = maxResults
	}
	path := fmt.Sprintf("/v1/data_sources/%s/query", db.DataSourceID)
	var resp queryResponse
	if err := c.doJSON(ctx, http.MethodPost, path, body, &resp, notionVersion); err != nil {
		return nil, err
	}
	return mapTasks(resp.Results, db.TitlePropertyName, db.StatusPropertyName, ""), nil
}

// TaskQueryResult はタスクの問い合わせの結果。
type TaskQueryResult struct {
	Tasks     []dto.Task
	Truncated bool
}

// QueryTasks は v2 のタスクの問い合わせ。期日つきの設定なら
// 「進行中 または（期日 ≦ 今日＋8 日 かつ 完了・中断でない）」を全件取り、
// そうでなければ v1 と同じ「進行中のみ」（max_results 件まで）を取る。
// now は「今日」の基準で、期日は now のタイムゾーンの日付に直す。
func (c *Client) QueryTasks(ctx context.Context, db dto.DatabaseConfig, notionVersion string, maxResults int, now time.Time) (TaskQueryResult, error) {
	if !db.UsesDueDateQuery() {
		tasks, err := c.QueryByStatus(ctx, db, notionVersion, maxResults, db.StatusInProgress)
		return TaskQueryResult{Tasks: tasks}, err
	}
	if err := db.ValidateForTaskQuery(notionVersion, db.StatusInProgress); err != nil {
		return TaskQueryResult{}, err
	}
	until := now.AddDate(0, 0, dueDateLookaheadDays).Format(dateOnlyLayout)
	body := map[string]any{
		"filter": buildDueTaskFilter(db, until),
		"sorts": []map[string]any{{
			"timestamp": "last_edited_time",
			"direction": "descending",
		}},
	}
	pages, truncated, err := c.queryAll(ctx, db.DataSourceID, body, notionVersion)
	if err != nil {
		return TaskQueryResult{}, err
	}
	fields := taskFields{
		Title:    db.TitlePropertyName,
		Status:   db.StatusPropertyName,
		DueDate:  db.DueDatePropertyName,
		Priority: db.PriorityPropertyName,
	}
	return TaskQueryResult{Tasks: mapTaskPages(pages, fields, now.Location()), Truncated: truncated}, nil
}

// queryAll はページ送りで全件を取る。maxQueryPages で打ち切ったら truncated を true にする。
// 途中のページで失敗したら、途中までの結果は返さない。
func (c *Client) queryAll(ctx context.Context, dataSourceID string, body map[string]any, notionVersion string) ([]page, bool, error) {
	path := fmt.Sprintf("/v1/data_sources/%s/query", dataSourceID)
	var all []page
	cursor := ""
	for i := 0; i < maxQueryPages; i++ {
		req := make(map[string]any, len(body)+2)
		for k, v := range body {
			req[k] = v
		}
		req["page_size"] = queryPageSize
		if cursor != "" {
			req["start_cursor"] = cursor
		}
		var resp queryResponse
		if err := c.doJSON(ctx, http.MethodPost, path, req, &resp, notionVersion); err != nil {
			return nil, false, err
		}
		all = append(all, resp.Results...)
		if !resp.HasMore || resp.NextCursor == "" {
			return all, false, nil
		}
		cursor = resp.NextCursor
	}
	return all, true, nil
}

func (c *Client) UpdateStatus(ctx context.Context, pageID string, db dto.DatabaseConfig, notionVersion string, statusValue string) error {
	if err := db.ValidateForTaskQuery(notionVersion, statusValue); err != nil {
		return err
	}
	body := map[string]any{
		"properties": map[string]any{
			db.StatusPropertyName: buildStatusUpdate(db.StatusPropertyType, statusValue),
		},
	}
	path := fmt.Sprintf("/v1/pages/%s", pageID)
	return c.doJSON(ctx, http.MethodPatch, path, body, nil, notionVersion)
}

func (c *Client) QueryHabitsToday(ctx context.Context, db dto.DatabaseConfig, checkboxPropertyName string, notionVersion string, maxResults int) ([]dto.Task, error) {
	if err := db.ValidateForHabit(notionVersion); err != nil {
		return nil, err
	}
	if checkboxPropertyName == "" {
		return nil, fmt.Errorf("checkbox_property_name is required")
	}
	body := map[string]any{
		"sorts": []map[string]any{{
			"timestamp": "created_time",
			"direction": "descending",
		}},
	}
	if maxResults > 0 {
		body["page_size"] = maxResults
	}
	path := fmt.Sprintf("/v1/data_sources/%s/query", db.DataSourceID)
	var resp queryResponse
	if err := c.doJSON(ctx, http.MethodPost, path, body, &resp, notionVersion); err != nil {
		return nil, err
	}
	return mapTasks(resp.Results, db.TitlePropertyName, "", checkboxPropertyName), nil
}

func (c *Client) UpdateCheckbox(ctx context.Context, pageID string, db dto.DatabaseConfig, checkboxPropertyName string, notionVersion string, checked bool) error {
	if err := db.ValidateForHabit(notionVersion); err != nil {
		return err
	}
	if checkboxPropertyName == "" {
		return fmt.Errorf("checkbox_property_name is required")
	}
	body := map[string]any{
		"properties": map[string]any{
			checkboxPropertyName: map[string]any{
				"checkbox": checked,
			},
		},
	}
	path := fmt.Sprintf("/v1/pages/%s", pageID)
	return c.doJSON(ctx, http.MethodPatch, path, body, nil, notionVersion)
}

func (c *Client) ResolveDataSourceID(ctx context.Context, databaseID string, notionVersion string) (string, error) {
	if databaseID == "" {
		return "", fmt.Errorf("database_id is required")
	}
	var resp databaseResponse
	path := fmt.Sprintf("/v1/databases/%s", databaseID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp, notionVersion); err != nil {
		return "", err
	}
	if len(resp.DataSources) == 0 {
		return "", fmt.Errorf("no data_sources found in database")
	}
	if len(resp.DataSources) > 1 {
		return "", fmt.Errorf("multiple data_sources found; only one is supported")
	}
	return resp.DataSources[0].ID, nil
}

func (c *Client) ResolveTitlePropertyName(ctx context.Context, databaseID string, notionVersion string) (string, error) {
	props, err := c.FetchDatabaseProperties(ctx, databaseID, notionVersion)
	if err != nil {
		return "", err
	}
	for _, prop := range props {
		if prop.Type == "title" {
			return prop.Name, nil
		}
	}
	return "", fmt.Errorf("title property not found")
}

func (c *Client) FetchDatabaseProperties(ctx context.Context, databaseID string, notionVersion string) ([]dto.DatabaseProperty, error) {
	if databaseID == "" {
		return nil, fmt.Errorf("database_id is required")
	}
	version := c.effectiveNotionVersion(notionVersion)
	if version >= defaultNotionVersion {
		dataSourceID, err := c.ResolveDataSourceID(ctx, databaseID, version)
		if err != nil {
			return nil, err
		}
		return c.fetchDataSourceProperties(ctx, dataSourceID, version)
	}
	return c.fetchDatabaseProperties(ctx, databaseID, version)
}

func (c *Client) fetchDatabaseProperties(ctx context.Context, databaseID string, notionVersion string) ([]dto.DatabaseProperty, error) {
	var resp databaseResponse
	path := fmt.Sprintf("/v1/databases/%s", databaseID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp, notionVersion); err != nil {
		return nil, err
	}
	return mapProperties(resp.Properties), nil
}

func (c *Client) fetchDataSourceProperties(ctx context.Context, dataSourceID string, notionVersion string) ([]dto.DatabaseProperty, error) {
	if strings.TrimSpace(dataSourceID) == "" {
		return nil, fmt.Errorf("data_source_id is required")
	}
	var resp dataSourceResponse
	path := fmt.Sprintf("/v1/data_sources/%s", dataSourceID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp, notionVersion); err != nil {
		return nil, err
	}
	return mapProperties(resp.Properties), nil
}

func mapProperties(props map[string]propertySchema) []dto.DatabaseProperty {
	out := make([]dto.DatabaseProperty, 0, len(props))
	for name, prop := range props {
		item := dto.DatabaseProperty{
			Name: name,
			Type: prop.Type,
		}
		switch prop.Type {
		case "status":
			if prop.Status != nil {
				item.Options = extractOptionNames(prop.Status.Options)
			}
		case "select":
			if prop.Select != nil {
				item.Options = extractOptionNames(prop.Select.Options)
			}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func extractOptionNames(options []databaseOption) []string {
	if len(options) == 0 {
		return nil
	}
	out := make([]string, 0, len(options))
	for _, opt := range options {
		if strings.TrimSpace(opt.Name) == "" {
			continue
		}
		out = append(out, opt.Name)
	}
	return out
}

func buildStatusFilter(name, typ, value string) map[string]any {
	return buildStatusCondition(name, typ, "equals", value)
}

// buildStatusCondition はステータスの条件を、プロパティの型（status / select）に合わせたキーで組む。
func buildStatusCondition(name, typ, op, value string) map[string]any {
	key := "status"
	if typ == "select" {
		key = "select"
	}
	return map[string]any{
		"property": name,
		key:        map[string]any{op: value},
	}
}

// buildDueTaskFilter は「進行中 または（期日 ≦ until かつ 完了でない かつ 中断でない）」を組む。
// Notion の複合フィルタの上限（2 段）に収まる。
func buildDueTaskFilter(db dto.DatabaseConfig, until string) map[string]any {
	name, typ := db.StatusPropertyName, db.StatusPropertyType
	return map[string]any{
		"or": []map[string]any{
			buildStatusCondition(name, typ, "equals", db.StatusInProgress),
			{
				"and": []map[string]any{
					{
						"property": db.DueDatePropertyName,
						"date":     map[string]any{"on_or_before": until},
					},
					buildStatusCondition(name, typ, "does_not_equal", db.StatusDone),
					buildStatusCondition(name, typ, "does_not_equal", db.StatusPaused),
				},
			},
		},
	}
}

func buildStatusUpdate(typ, value string) map[string]any {
	if typ == "select" {
		return map[string]any{
			"select": map[string]any{"name": value},
		}
	}
	return map[string]any{
		"status": map[string]any{"name": value},
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any, notionVersion string) error {
	token, err := c.tokenStore.GetToken()
	if err != nil {
		if errors.Is(err, store.ErrTokenNotFound) {
			return errors.New("notion token is not set")
		}
		return err
	}
	if token == "" {
		return errors.New("notion token is empty")
	}
	notionVersion = c.effectiveNotionVersion(notionVersion)
	if notionVersion == "" {
		return errors.New("notion_version is empty")
	}

	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		payload = b
	}

	url := c.baseURL + path

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		var buf io.Reader
		if payload != nil {
			buf = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, buf)
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Notion-Version", notionVersion)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http do: %w", err)
			if attempt == c.maxRetries {
				return lastErr
			}
			time.Sleep(c.retryWait)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out == nil {
				resp.Body.Close()
				return nil
			}
			if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
				resp.Body.Close()
				return fmt.Errorf("decode response: %w", err)
			}
			resp.Body.Close()
			return nil
		}

		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if shouldRetry(resp.StatusCode) && attempt < c.maxRetries {
			wait := c.retryWait
			if resp.StatusCode == http.StatusTooManyRequests {
				wait = retryAfter(resp.Header.Get("Retry-After"), wait)
			}
			time.Sleep(wait)
			lastErr = fmt.Errorf("notion error: %s", strings.TrimSpace(string(b)))
			continue
		}
		return newAPIError(resp.StatusCode, b)
	}
	return lastErr
}

func (c *Client) effectiveNotionVersion(version string) string {
	if strings.TrimSpace(version) == "" {
		return c.version
	}
	if c.version != "" && version < c.version {
		return c.version
	}
	return version
}

func shouldRetry(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return status >= 500
}

func retryAfter(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	sec, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if sec <= 0 {
		return fallback
	}
	return time.Duration(sec) * time.Second
}
