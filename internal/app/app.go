package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
	"nudge/internal/store"
	syncer "nudge/internal/sync"
)

type App struct {
	cfgStore   store.ConfigStore
	tokenStore store.TokenStore
	notion     *notion.Client
	poller     *syncer.Poller

	mu  sync.Mutex
	cfg dto.Config
}

func NewApp(cfgStore store.ConfigStore, tokenStore store.TokenStore, notionClient *notion.Client) *App {
	return &App{
		cfgStore:   cfgStore,
		tokenStore: tokenStore,
		notion:     notionClient,
	}
}

func (a *App) LoadConfig() (dto.Config, error) {
	cfg, err := a.cfgStore.Load()
	if err != nil {
		return cfg, err
	}
	cfg = cfg.Normalize()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
	return cfg, nil
}

func (a *App) GetConfig() dto.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *App) SaveConfig(cfg dto.Config) error {
	cfg = cfg.Normalize()
	prev := a.currentConfig()
	if prev.LaunchAtLogin != cfg.LaunchAtLogin {
		if err := setLaunchAtLogin(cfg.LaunchAtLogin); err != nil {
			return err
		}
	}
	if err := a.cfgStore.Save(cfg); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return nil
}

func (a *App) GetToken() (string, error) {
	return a.tokenStore.GetToken()
}

func (a *App) SetToken(token string) error {
	if token == "" {
		return fmt.Errorf("token is empty")
	}
	return a.tokenStore.SetToken(token)
}

func (a *App) ClearToken() error {
	return a.tokenStore.ClearToken()
}

func (a *App) RefreshTasks(ctx context.Context) ([]dto.Task, error) {
	return a.QueryTasks(ctx, "")
}

func (a *App) UpdateTaskStatus(ctx context.Context, databaseKey, taskID string, action string) error {
	if taskID == "" {
		return fmt.Errorf("taskID is empty")
	}
	db, cfg, err := a.resolveDatabase(databaseKey, dto.DatabaseKindTask)
	if err != nil {
		return err
	}
	if db.Kind != dto.DatabaseKindTask {
		return fmt.Errorf("database kind is not task")
	}
	statusValue := db.StatusForAction(action)
	if statusValue == "" {
		return fmt.Errorf("status is not configured")
	}
	return a.notion.UpdateStatus(ctx, taskID, db, cfg.NotionVersion, statusValue)
}

func (a *App) QueryTasks(ctx context.Context, databaseKey string) ([]dto.Task, error) {
	db, cfg, err := a.resolveDatabase(databaseKey, dto.DatabaseKindTask)
	if err != nil {
		return nil, err
	}
	if db.Kind != dto.DatabaseKindTask {
		return nil, fmt.Errorf("database kind is not task")
	}
	return a.notion.QueryByStatus(ctx, db, cfg.NotionVersion, cfg.MaxResults, db.StatusInProgress)
}

func (a *App) QueryHabits(ctx context.Context, databaseKey string) ([]dto.Task, error) {
	db, cfg, err := a.resolveDatabase(databaseKey, dto.DatabaseKindHabit)
	if err != nil {
		return nil, err
	}
	if db.Kind != dto.DatabaseKindHabit {
		return nil, fmt.Errorf("database kind is not habit")
	}
	db, err = a.ensureHabitDatabase(ctx, db, cfg.NotionVersion)
	if err != nil {
		return nil, err
	}
	checkboxPropertyName, err := resolveHabitCheckboxProperty(db, time.Now())
	if err != nil {
		return nil, err
	}
	tasks, err := a.notion.QueryHabitsToday(ctx, db, checkboxPropertyName, cfg.NotionVersion, cfg.MaxResults)
	if err != nil {
		return nil, err
	}
	return filterUnchecked(uniqueTasksByTitle(tasks)), nil
}

func (a *App) UpdateHabitCheck(ctx context.Context, databaseKey, taskID string, checked bool) error {
	if taskID == "" {
		return fmt.Errorf("taskID is empty")
	}
	db, cfg, err := a.resolveDatabase(databaseKey, dto.DatabaseKindHabit)
	if err != nil {
		return err
	}
	if db.Kind != dto.DatabaseKindHabit {
		return fmt.Errorf("database kind is not habit")
	}
	db, err = a.ensureHabitDatabase(ctx, db, cfg.NotionVersion)
	if err != nil {
		return err
	}
	checkboxPropertyName, err := resolveHabitCheckboxProperty(db, time.Now())
	if err != nil {
		return err
	}
	return a.notion.UpdateCheckbox(ctx, taskID, db, checkboxPropertyName, cfg.NotionVersion, checked)
}

func (a *App) ResolveDataSourceID(ctx context.Context, databaseID string) (string, error) {
	cfg := a.currentConfig()
	return a.notion.ResolveDataSourceID(ctx, databaseID, cfg.NotionVersion)
}

func (a *App) ResolveTitlePropertyName(ctx context.Context, databaseID string) (string, error) {
	cfg := a.currentConfig()
	return a.notion.ResolveTitlePropertyName(ctx, databaseID, cfg.NotionVersion)
}

func (a *App) StartPolling(ctx context.Context, refresh func([]dto.Task)) error {
	a.StopPolling()
	cfg := a.currentConfig()
	interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	p := &syncer.Poller{
		Interval: interval,
		Refresh: func(ctx context.Context) error {
			tasks, err := a.RefreshTasks(ctx)
			if err != nil {
				return err
			}
			if refresh != nil {
				refresh(tasks)
			}
			return nil
		},
	}
	p.Start(ctx)
	a.poller = p
	return nil
}

func (a *App) StopPolling() {
	if a.poller == nil {
		return
	}
	a.poller.Stop()
}

func (a *App) currentConfig() dto.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

func (a *App) resolveDatabase(key, kind string) (dto.DatabaseConfig, dto.Config, error) {
	cfg := a.currentConfig()
	if key == "" {
		if db, ok := cfg.FirstDatabaseByKind(kind); ok {
			if !db.Enabled {
				return dto.DatabaseConfig{}, cfg, fmt.Errorf("database is disabled")
			}
			return db, cfg, nil
		}
		return dto.DatabaseConfig{}, cfg, fmt.Errorf("database is not configured")
	}
	db, ok := cfg.DatabaseByKey(key)
	if !ok {
		return dto.DatabaseConfig{}, cfg, fmt.Errorf("database not found")
	}
	if !db.Enabled {
		return dto.DatabaseConfig{}, cfg, fmt.Errorf("database is disabled")
	}
	return db, cfg, nil
}

func resolveHabitCheckboxProperty(db dto.DatabaseConfig, now time.Time) (string, error) {
	raw := strings.TrimSpace(db.CheckboxPropertyName)
	if raw == "" {
		raw = dto.DefaultHabitDays
	}
	parts := splitAndTrim(raw, ",")
	if len(parts) == 1 {
		return parts[0], nil
	}
	weekday := int(now.Weekday())
	if weekday >= 0 && weekday < len(parts) && parts[weekday] != "" {
		return parts[weekday], nil
	}
	return parts[0], nil
}

func splitAndTrim(value, sep string) []string {
	chunks := strings.Split(value, sep)
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(value)}
	}
	return out
}

func uniqueTasksByTitle(tasks []dto.Task) []dto.Task {
	out := make([]dto.Task, 0, len(tasks))
	seen := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		key := strings.TrimSpace(task.Title)
		if key == "" {
			key = task.ID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, task)
	}
	return out
}

func filterUnchecked(tasks []dto.Task) []dto.Task {
	out := make([]dto.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Checked {
			continue
		}
		out = append(out, task)
	}
	return out
}

func (a *App) ensureHabitDatabase(ctx context.Context, db dto.DatabaseConfig, notionVersion string) (dto.DatabaseConfig, error) {
	if db.DataSourceID == "" {
		if strings.TrimSpace(db.DatabaseID) == "" {
			return db, fmt.Errorf("database_id is required")
		}
		id, err := a.notion.ResolveDataSourceID(ctx, db.DatabaseID, notionVersion)
		if err != nil {
			return db, err
		}
		db.DataSourceID = id
	}
	if strings.TrimSpace(db.TitlePropertyName) == "" {
		if strings.TrimSpace(db.DatabaseID) == "" {
			return db, fmt.Errorf("database_id is required")
		}
		name, err := a.notion.ResolveTitlePropertyName(ctx, db.DatabaseID, notionVersion)
		if err != nil {
			return db, err
		}
		db.TitlePropertyName = name
	}
	return db, nil
}
