package app

import (
	"context"
	"fmt"
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
	cfg := a.currentConfig()
	return a.notion.QueryInProgress(ctx, cfg)
}

func (a *App) UpdateTaskStatus(ctx context.Context, taskID string, statusValue string) error {
	if taskID == "" {
		return fmt.Errorf("taskID is empty")
	}
	cfg := a.currentConfig()
	return a.notion.UpdateStatus(ctx, taskID, cfg, statusValue)
}

func (a *App) QueryByStatus(ctx context.Context, statusValue string) ([]dto.Task, error) {
	cfg := a.currentConfig()
	return a.notion.QueryByStatus(ctx, cfg, statusValue)
}

func (a *App) ResolveDataSourceID(ctx context.Context, databaseID string) (string, error) {
	cfg := a.currentConfig()
	cfg.DatabaseID = databaseID
	return a.notion.ResolveDataSourceID(ctx, cfg)
}

func (a *App) ResolveTitlePropertyName(ctx context.Context, databaseID string) (string, error) {
	cfg := a.currentConfig()
	cfg.DatabaseID = databaseID
	return a.notion.ResolveTitlePropertyName(ctx, cfg)
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
