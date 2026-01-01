package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nudge/internal/dto"
	"nudge/internal/store"
)

const (
	defaultBaseURL = "https://api.notion.com"
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
	if databaseID == "" {
		return "", fmt.Errorf("database_id is required")
	}
	var resp databaseResponse
	path := fmt.Sprintf("/v1/databases/%s", databaseID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp, notionVersion); err != nil {
		return "", err
	}
	for name, prop := range resp.Properties {
		if prop.Type == "title" {
			return name, nil
		}
	}
	return "", fmt.Errorf("title property not found")
}

func buildStatusFilter(name, typ, value string) map[string]any {
	if typ == "select" {
		return map[string]any{
			"property": name,
			"select":   map[string]any{"equals": value},
		}
	}
	return map[string]any{
		"property": name,
		"status":   map[string]any{"equals": value},
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
	if notionVersion == "" {
		notionVersion = c.version
	}
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
		return fmt.Errorf("notion error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return lastErr
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
