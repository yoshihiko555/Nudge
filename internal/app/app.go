package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
	"nudge/internal/store"
	syncer "nudge/internal/sync"
)

type App struct {
	cfgStore     store.ConfigStore
	tokenStore   store.TokenStore
	notion       *notion.Client
	poller       *syncer.Poller
	pollerCancel context.CancelFunc
	pollerMu     sync.Mutex
	refreshMu    sync.Mutex
	// cacheMu はキャッシュ・取得結果・設定の世代番号を守る（詳細設計 5.5）。
	cacheMu    sync.Mutex
	taskCache  map[string][]dto.Task
	habitCache map[string][]dto.Task
	syncStates map[string]syncRecord
	// generation は設定を保存するたびに増える。取得を始めた時と違えば、その結果は書かない。
	generation uint64
	// ready は起動後の最初の取得が終わったか。reloading は設定の保存後の取り直しの途中か。
	ready     bool
	reloading bool
	// notifier は件数が変わりうるときに合図を送る先（メニューバーのラベル）。
	notifier chan<- struct{}

	now func() time.Time
	// loc は「今日」「週」を判定するタイムゾーン（既定は Mac の設定）。テストでは固定する。
	loc *time.Location

	mu  sync.Mutex
	cfg dto.Config
}

func NewApp(cfgStore store.ConfigStore, tokenStore store.TokenStore, notionClient *notion.Client) *App {
	return &App{
		cfgStore:   cfgStore,
		tokenStore: tokenStore,
		notion:     notionClient,
		taskCache:  make(map[string][]dto.Task),
		habitCache: make(map[string][]dto.Task),
		syncStates: make(map[string]syncRecord),
		now:        time.Now,
		loc:        time.Local,
	}
}

// localNow は判定に使う現在時刻（詳細設計 1.1 の now().In(time.Local)）。
func (a *App) localNow() time.Time {
	return a.now().In(a.loc)
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
	a.invalidateCaches()
	return nil
}

// invalidateCaches は設定の保存のあと、古い設定で取ったデータと取得結果を捨てる（詳細設計 5.4）。
func (a *App) invalidateCaches() {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.generation++
	a.reloading = true
	a.taskCache = make(map[string][]dto.Task)
	a.habitCache = make(map[string][]dto.Task)
	a.syncStates = make(map[string]syncRecord)
}

func (a *App) GetToken() (string, error) {
	return a.tokenStore.GetToken()
}

func (a *App) IsConfigured() bool {
	token, err := a.tokenStore.GetToken()
	if err != nil || strings.TrimSpace(token) == "" {
		return false
	}
	cfg := a.currentConfig()
	return hasConfiguredDatabase(cfg)
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
	db, err = a.ensureTaskDatabase(ctx, db, cfg.NotionVersion)
	if err != nil {
		return err
	}
	statusValue := db.StatusForAction(action)
	if statusValue == "" {
		return fmt.Errorf("status is not configured")
	}
	return a.notion.UpdateStatus(ctx, taskID, db, cfg.NotionVersion, statusValue)
}

// QueryTasks はタスクを取得し、今の日付でまとまりを付けて並べ替えた結果を返す（キャッシュは使わない）。
func (a *App) QueryTasks(ctx context.Context, databaseKey string) ([]dto.Task, error) {
	db, res, err := a.fetchTasks(ctx, databaseKey)
	if err != nil {
		return nil, err
	}
	return buildTaskView(res.Tasks, db, a.localNow()), nil
}

func (a *App) fetchTasks(ctx context.Context, databaseKey string) (dto.DatabaseConfig, notion.TaskQueryResult, error) {
	db, cfg, err := a.resolveDatabase(databaseKey, dto.DatabaseKindTask)
	if err != nil {
		return db, notion.TaskQueryResult{}, err
	}
	if db.Kind != dto.DatabaseKindTask {
		return db, notion.TaskQueryResult{}, fmt.Errorf("database kind is not task")
	}
	db, err = a.ensureTaskDatabase(ctx, db, cfg.NotionVersion)
	if err != nil {
		return db, notion.TaskQueryResult{}, err
	}
	res, err := a.notion.QueryTasks(ctx, db, cfg.NotionVersion, cfg.MaxResults, a.localNow())
	return db, res, err
}

// GetTasks はタスクのまとまりと並び順を、呼ばれた時点の日付で計算して返す。
// force ならその場で取り直す。取り直しに失敗してもキャッシュがあれば、エラーにせずキャッシュを返す
// （失敗は GetSyncState に載る。NFR-04）。キャッシュが無いときだけエラーを返す。
func (a *App) GetTasks(ctx context.Context, databaseKey string, force bool) ([]dto.Task, error) {
	db, _, err := a.resolveDatabase(databaseKey, dto.DatabaseKindTask)
	if err != nil {
		return nil, err
	}
	if db.Kind != dto.DatabaseKindTask {
		return nil, fmt.Errorf("database kind is not task")
	}
	if !force {
		if tasks, ok := a.getTaskCache(databaseKey); ok {
			return buildTaskView(tasks, db, a.localNow()), nil
		}
	}
	err = a.forceRefresh(func() (bool, error) { return a.refreshTaskDatabase(ctx, databaseKey) })
	a.notifySummary()
	tasks, ok := a.getTaskCache(databaseKey)
	if !ok {
		if err == nil {
			err = errStaleGeneration
		}
		return nil, err
	}
	return buildTaskView(tasks, db, a.localNow()), nil
}

// errStaleGeneration は、取り直しの途中で設定が保存され続けて結果を書けなかったことを表す。
var errStaleGeneration = fmt.Errorf("settings changed during refresh; please retry")

// forceRefresh は手動更新・操作後の取り直しを、ポーリング（refreshAll）と同時に走らせずに行う。
// 途中で設定が保存されて結果を捨てた（書けなかった）ときは、新しい設定で 1 回だけやり直す。
func (a *App) forceRefresh(refresh func() (bool, error)) error {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	written, err := refresh()
	if err == nil && !written {
		_, err = refresh()
	}
	return err
}

// refreshTaskDatabase は 1 つのタスク DB を取り直し、キャッシュと取得結果を更新する。
// 途中で設定が保存されて結果を捨てたときは written が false になる。呼び出し側が refreshMu を持つ。
func (a *App) refreshTaskDatabase(ctx context.Context, databaseKey string) (written bool, err error) {
	gen := a.currentGeneration()
	_, res, err := a.fetchTasks(ctx, databaseKey)
	return a.storeResult(dto.DatabaseKindTask, databaseKey, gen, res.Tasks, res.Truncated, err), err
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

// GetHabits は今日の未チェックの習慣を返す。失敗時の返し方は GetTasks と同じ。
func (a *App) GetHabits(ctx context.Context, databaseKey string, force bool) ([]dto.Task, error) {
	if !force {
		if habits, ok := a.getHabitCache(databaseKey); ok {
			return habits, nil
		}
	}
	err := a.forceRefresh(func() (bool, error) { return a.refreshHabitDatabase(ctx, databaseKey) })
	a.notifySummary()
	habits, ok := a.getHabitCache(databaseKey)
	if !ok {
		if err == nil {
			err = errStaleGeneration
		}
		return nil, err
	}
	return habits, nil
}

// refreshHabitDatabase は 1 つの習慣 DB を取り直し、キャッシュと取得結果を更新する。
// 途中で設定が保存されて結果を捨てたときは written が false になる。呼び出し側が refreshMu を持つ。
func (a *App) refreshHabitDatabase(ctx context.Context, databaseKey string) (written bool, err error) {
	gen := a.currentGeneration()
	habits, err := a.QueryHabits(ctx, databaseKey)
	return a.storeResult(dto.DatabaseKindHabit, databaseKey, gen, habits, false, err), err
}

// GetSyncState は有効な DB ごとの最終取得の結果を、設定の並び順で返す（API-006）。
func (a *App) GetSyncState() []dto.SyncState {
	cfg := a.currentConfig()
	a.cacheMu.Lock()
	records := make(map[string]syncRecord, len(a.syncStates))
	for k, v := range a.syncStates {
		records[k] = v
	}
	a.cacheMu.Unlock()

	out := make([]dto.SyncState, 0, len(cfg.Databases))
	for _, db := range cfg.Databases {
		if !db.Enabled {
			continue
		}
		out = append(out, records[db.Key].toDTO(db))
	}
	return out
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

func (a *App) GetDatabaseProperties(ctx context.Context, databaseID string) ([]dto.DatabaseProperty, error) {
	cfg := a.currentConfig()
	return a.notion.FetchDatabaseProperties(ctx, databaseID, cfg.NotionVersion)
}

func (a *App) StartPolling(ctx context.Context, refresh func([]dto.Task)) error {
	a.pollerMu.Lock()
	defer a.pollerMu.Unlock()
	a.stopPollingLocked()
	cfg := a.currentConfig()
	interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	p := &syncer.Poller{
		Interval: interval,
		Logger:   slog.Default(),
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

func (a *App) StartBackgroundPolling() {
	a.pollerMu.Lock()
	defer a.pollerMu.Unlock()
	a.stopPollingLocked()

	cfg := a.currentConfig()
	interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())
	p := &syncer.Poller{
		Interval: interval,
		Logger:   slog.Default(),
		Refresh: func(ctx context.Context) error {
			return a.refreshAll(ctx)
		},
	}
	a.poller = p
	a.pollerCancel = cancel

	go func() {
		_ = a.refreshAll(ctx)
	}()
	p.Start(ctx)
}

func (a *App) StopPolling() {
	a.pollerMu.Lock()
	defer a.pollerMu.Unlock()
	a.stopPollingLocked()
}

func (a *App) stopPollingLocked() {
	if a.poller == nil {
		return
	}
	a.poller.Stop()
	a.poller = nil
	if a.pollerCancel != nil {
		a.pollerCancel()
		a.pollerCancel = nil
	}
}

func (a *App) currentConfig() dto.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// refreshAll は有効な全 DB を取り直し、終わったら件数の合図を送る（詳細設計 5.1）。
func (a *App) refreshAll(ctx context.Context) error {
	err := a.refreshAllLocked(ctx)
	a.notifySummary()
	return err
}

func (a *App) refreshAllLocked(ctx context.Context) error {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()

	gen := a.currentGeneration()
	defer a.markRefreshed(gen)
	cfg := a.currentConfig()
	var firstErr error
	for _, db := range cfg.Databases {
		if !db.Enabled {
			continue
		}
		var err error
		switch db.Kind {
		case dto.DatabaseKindHabit:
			_, err = a.refreshHabitDatabase(ctx, db.Key)
		default:
			_, err = a.refreshTaskDatabase(ctx, db.Key)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (a *App) getTaskCache(key string) ([]dto.Task, bool) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	tasks, ok := a.taskCache[key]
	if !ok {
		return nil, false
	}
	return cloneTasks(tasks), true
}

// markRefreshed は全 DB の取得が一巡したことを記録する。始めた時から設定が変わっていなければ、
// 設定の保存後の取り直しも終わったとみなす。
func (a *App) markRefreshed(gen uint64) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	a.ready = true
	if gen == a.generation {
		a.reloading = false
	}
}

func (a *App) currentGeneration() uint64 {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	return a.generation
}

// storeResult は取得の結果をキャッシュと取得結果に書き、書いたかどうかを返す。取得を始めた時の世代と
// 今の世代が違えば（途中で設定が保存されたら）何も書かずに false を返す。失敗したときはキャッシュを残す。
func (a *App) storeResult(kind, key string, gen uint64, items []dto.Task, truncated bool, err error) bool {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if gen != a.generation {
		return false
	}
	now := a.now()
	rec := a.syncStates[key]
	rec.attempted = true
	rec.lastAttempt = now
	if err != nil {
		rec.ok = false
		rec.errorKind = classifyError(err)
		// 失敗は DB 名と種類をログに残す（NFR-06）。トークンはエラーに含まれない。
		slog.Warn("notion fetch failed", "database", key, "kind", rec.errorKind, "error", err)
		a.syncStates[key] = rec
		return true
	}
	rec.ok = true
	rec.lastSuccess = now
	rec.errorKind = ""
	rec.truncated = truncated
	a.syncStates[key] = rec
	if kind == dto.DatabaseKindHabit {
		a.habitCache[key] = cloneTasks(items)
		return true
	}
	a.taskCache[key] = cloneTasks(items)
	return true
}

func (a *App) getHabitCache(key string) ([]dto.Task, bool) {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	habits, ok := a.habitCache[key]
	if !ok {
		return nil, false
	}
	return cloneTasks(habits), true
}

func cloneTasks(tasks []dto.Task) []dto.Task {
	if tasks == nil {
		return nil
	}
	out := make([]dto.Task, len(tasks))
	copy(out, tasks)
	return out
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
		return "", fmt.Errorf("checkbox_property_name is not configured")
	}
	parts := splitAndTrim(raw, ",")
	if len(parts) == 1 {
		return parts[0], nil
	}
	weekday := int(now.Weekday())
	if weekday < 0 || weekday >= len(parts) {
		return "", fmt.Errorf("checkbox_property_name has %d entries but weekday index is %d", len(parts), weekday)
	}
	if parts[weekday] == "" {
		return "", fmt.Errorf("checkbox_property_name entry for weekday %d is empty", weekday)
	}
	return parts[weekday], nil
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

func (a *App) ensureTaskDatabase(ctx context.Context, db dto.DatabaseConfig, notionVersion string) (dto.DatabaseConfig, error) {
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

func hasConfiguredDatabase(cfg dto.Config) bool {
	for _, db := range cfg.Databases {
		if !db.Enabled {
			continue
		}
		hasID := strings.TrimSpace(db.DatabaseID) != "" || strings.TrimSpace(db.DataSourceID) != ""
		if !hasID {
			continue
		}
		switch db.Kind {
		case dto.DatabaseKindHabit:
			if strings.TrimSpace(db.TitlePropertyName) != "" || strings.TrimSpace(db.DatabaseID) != "" {
				return true
			}
		default:
			if strings.TrimSpace(db.StatusPropertyName) == "" || strings.TrimSpace(db.StatusInProgress) == "" {
				continue
			}
			if strings.TrimSpace(db.TitlePropertyName) != "" || strings.TrimSpace(db.DatabaseID) != "" {
				return true
			}
		}
	}
	return false
}
