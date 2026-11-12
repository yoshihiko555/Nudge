package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

// historyRetryWaits は過去分の取得に失敗したときの再試行の間隔（以後は最後の値を繰り返す）。
var historyRetryWaits = []time.Duration{5 * time.Minute, 15 * time.Minute, 60 * time.Minute}

// errHistoryTruncated は過去分が件数の上限で欠けたことを表す。欠けた週は「ページが無い週」として飛ばされ、
// 連続達成週数が多く出てしまうので、失敗として扱い前のキャッシュを残す。
var errHistoryTruncated = errors.New("habit history truncated by the page limit")

// historyState は過去分の取得の状態（historyMu で守る）。
type historyState struct {
	fetchedFor   string
	running      bool
	pending      bool
	pendingForce bool
	nextRetryAt  time.Time
	retryStep    int
}

// historyRecord は DB ごとの過去分の取得結果（cacheMu で守る）。
type historyRecord struct {
	loaded     bool
	pages      []notion.HabitPage
	attempted  bool
	ok         bool
	errorKind  string
	fetchedFor string
}

// RequestHistoryRefresh は過去分を強制で取り直す（手動更新のとき。詳細設計 5.1）。
func (a *App) RequestHistoryRefresh() {
	a.requestHistory(true)
}

// requestHistory は過去分の取得を要求する。走っている最中なら終わった後に 1 回だけやり直す。
// 強制でないときは、今日すでに取れているか、再試行の時刻の前なら何もしない。
func (a *App) requestHistory(force bool) {
	now := a.localNow()
	a.historyMu.Lock()
	if a.history.running {
		a.history.pending = true
		a.history.pendingForce = a.history.pendingForce || force
		a.historyMu.Unlock()
		return
	}
	if !force && (a.history.fetchedFor == now.Format(dateLayout) || now.Before(a.history.nextRetryAt)) {
		a.historyMu.Unlock()
		return
	}
	if !a.hasWeeklyHabitDatabase() {
		a.historyMu.Unlock()
		return
	}
	a.history.running = true
	a.historyMu.Unlock()
	a.spawn(a.runHistoryFetch)
}

func (a *App) hasWeeklyHabitDatabase() bool {
	for _, db := range a.currentConfig().Databases {
		if db.Enabled && db.Kind == dto.DatabaseKindHabit && usesWeeklyHabits(db) {
			return true
		}
	}
	return false
}

// runHistoryFetch は週集計を使う習慣 DB の過去分を順に取り、DB ごとにキャッシュを丸ごと差し替える。
// refreshMu は持たないので、ポーリングや操作を待たせない。
func (a *App) runHistoryFetch() {
	ctx := context.Background()
	gen := a.currentGeneration()
	now := a.localNow()
	today := now.Format(dateLayout)
	allOK, stale := true, false
	for _, db := range a.currentConfig().Databases {
		if !db.Enabled || db.Kind != dto.DatabaseKindHabit || !usesWeeklyHabits(db) {
			continue
		}
		pages, err := a.fetchHabitHistory(ctx, db, now)
		if !a.storeHistory(db.Key, gen, pages, today, err) {
			// 途中で設定が保存された。保存の処理が強制で取り直しを要求しているので、再試行の間隔は変えない。
			stale = true
			continue
		}
		if err != nil {
			allOK = false
		}
	}
	a.finishHistoryFetch(today, allOK, stale)
	a.notifySummary()
}

func (a *App) fetchHabitHistory(ctx context.Context, db dto.DatabaseConfig, now time.Time) ([]notion.HabitPage, error) {
	cfg := a.currentConfig()
	db, err := a.ensureHabitDatabase(ctx, db, cfg.NotionVersion)
	if err != nil {
		return nil, err
	}
	thisWeek := weekStart(now)
	from := thisWeek.AddDate(0, 0, -daysPerWeek*historyWeeks)
	pages, truncated, err := a.notion.QueryHabitPages(ctx, db, cfg.NotionVersion, weekdayColumns(db), from, thisWeek)
	if err == nil && truncated {
		return nil, errHistoryTruncated
	}
	return pages, err
}

// storeHistory は過去分の結果を書く。世代が変わっていれば書かずに false を返す。失敗したら前のキャッシュを残す。
func (a *App) storeHistory(key string, gen uint64, pages []notion.HabitPage, today string, err error) bool {
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	if gen != a.generation {
		return false
	}
	rec := a.historyCache[key]
	rec.attempted = true
	if err != nil {
		rec.ok = false
		rec.errorKind = classifyError(err)
		a.historyCache[key] = rec
		slog.Warn("notion history fetch failed", "database", key, "kind", rec.errorKind, "error", err)
		return true
	}
	rec.ok = true
	rec.errorKind = ""
	rec.loaded = true
	rec.pages = pages
	rec.fetchedFor = today
	a.historyCache[key] = rec
	return true
}

// finishHistoryFetch は取得の後始末をする。全部取れたら今日の分は済み、失敗があれば間隔を空けて再試行する。
// stale（途中で設定が保存されて結果を捨てた）なら、成功とも失敗ともせず状態を変えない。
func (a *App) finishHistoryFetch(today string, allOK, stale bool) {
	a.historyMu.Lock()
	switch {
	case stale:
	case allOK:
		a.history.fetchedFor = today
		a.history.retryStep = 0
		a.history.nextRetryAt = time.Time{}
	default:
		wait := historyRetryWaits[a.history.retryStep]
		a.history.nextRetryAt = a.localNow().Add(wait)
		if a.history.retryStep < len(historyRetryWaits)-1 {
			a.history.retryStep++
		}
	}
	a.history.running = false
	rerun, force := a.history.pending, a.history.pendingForce
	a.history.pending, a.history.pendingForce = false, false
	a.historyMu.Unlock()
	if rerun {
		a.requestHistory(force)
	}
}

// resetHistoryForNewConfig は設定の保存のあと、過去分を「まだ取っていない」状態に戻す。
func (a *App) resetHistoryForNewConfig() {
	a.historyMu.Lock()
	a.history.fetchedFor = ""
	a.history.nextRetryAt = time.Time{}
	a.history.retryStep = 0
	a.historyMu.Unlock()
}
