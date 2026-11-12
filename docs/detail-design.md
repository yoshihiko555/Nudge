# 詳細設計

## モジュール分割（Go）

- `cmd/app`: エントリポイント
- `internal/app`: Wails アプリ初期化
- `internal/tray`: トレイ/ウィンドウ制御
- `internal/notion`: Notion API クライアント
- `internal/sync`: ポーリング/差分更新
- `internal/store`: 設定/Keychain/キャッシュ
- `internal/dto`: Task DTO, Config DTO
- `internal/log`: ログ初期化

依存: `tray` → `sync` → `notion` / `store`

## 主要インタフェース設計

### Task DTO（例）

- `id` string
- `title` string
- `url` string
- `last_edited_time` string (ISO8601)
- `status` string（内部利用のみ、UI非表示）

### NotionClient

- `QueryInProgress(ctx) ([]Task, error)`
- `UpdateStatus(ctx, pageID, statusValue) error`
- `ResolveDataSourceID(ctx, databaseID) (string, error)`

### ConfigStore

- `Load() (Config, error)`
- `Save(Config) error`

### TokenStore

- `GetToken() (string, error)`
- `SetToken(string) error`
- `ClearToken() error`

## Notion API 設計

### 使用エンドポイント（概要）

- Data Source Query
- Page Update
- Database Retrieve（初回に data_source_id を取得）

### 代表リクエスト例（JSON）

**Query（進行中）**

```json
{
  "filter": {
    "property": "<StatusPropertyName>",
    "status": { "equals": "<InProgressValue>" }
  },
  "sorts": [{ "timestamp": "last_edited_time", "direction": "descending" }]
}
```

**Update（完了/中断）**

```json
{
  "properties": {
    "<StatusPropertyName>": {
      "status": { "name": "<DoneOrPausedValue>" }
    }
  }
}
```

※ Status 型ではなく Select 型の場合は `status` を `select` に読み替える。

### フィルタ条件

> v2 で、期日のプロパティなどを設定した DB は取得の条件が変わった。「v2: 今日の絞り込みと習慣の週集計」の 2.2 と、ADR-20261112-0013 を参照。

- 進行中 = `<InProgressValue>`（取得対象は進行中のみ）
- 「中断」は `<PausedValue>` へ更新

### 例外・失敗時の扱い

- 401/403: トークン無効/権限不足 → 設定誘導
- 404: DB/データソース未共有
- 429: Retry-After に従いバックオフ
- 5xx/timeout: リトライ（指数バックオフ）

## Wails 連携設計

- Frontend → Backend: Wails Binding
- トレイ常駐: System Tray API でメニューバー常駐
- 小窓表示: トレイクリックで表示/非表示トグル
- ウィンドウ: フレームレス + 最前面（popover 風）
- 起動時: 設定未完了ならセットアップ画面を表示

## 画面仕様

### タスク一覧（進行中のみ）

> v2 で、まとまり（期限切れ・今日・進行中・直近 7 日）と 1 件 1 行の表示に変わった。「v2」の 7 章 SC-001 を参照。

- 表示項目: タイトル / 最終更新 / Notionリンク
- 最大件数: 30件（設定可）
- 並び順: last_edited_time 降順

### 中断一覧（メニューから切替表示）

- 表示項目: タイトル / 最終更新 / Notionリンク
- 最大件数: 30件（設定可）
- 操作: 「戻す」→ Status を `<InProgressValue>` へ更新

### 操作

- 完了: Status を `<DoneValue>` へ更新
- 中断: Status を `<PausedValue>` へ更新
- Notionで開く: URL を open

### バリデーション

- トークン未入力/無効
- database_id 形式不正
- status_property_name / status values 未設定

## テスト設計

### 単体テスト

- NotionClient を interface 化してモック化
- フィルタ生成、リトライ、バックオフの検証

### 結合テスト

- `httptest` で疑似 Notion API を構成

### 手動テスト観点

- ネットワーク断
- トークン無効
- 429（レート制限）
- 404（未共有）
- 5xx（サーバエラー）

## 前提 / 仮定 / 未決事項

### 前提

- Database は 1 つ

### 仮定

- Status プロパティ名/値は設定で与えられる

### 決定事項

- 中断タスクの「戻す」は「中断一覧」画面で提供する

---

## v2: 今日の絞り込みと習慣の週集計

- 作成日: 2026-11-12 / 状態: 詳細設計（実装済み: PR #3〜#7。決定は ADR-20261112-0013〜0016）
- 上流: `docs/overview.md`「v2」（要件）、`docs/basic-design.md`「v2」（基本設計: SC-001〜005 / API-001〜008 / N-01〜03）
- ここに書かない内部の名前（変数名・関数の分け方）は実装者が決めてよい。関数名は目安として書く

### 1. 共通の決めごと

#### 1.1 時刻と日付

- 現在時刻は `now func() time.Time` で注入する（既定は `time.Now`）。判定はすべて `now().In(time.Local)` で行う
- 「今日」はローカル時刻の年月日。比較は `YYYY-MM-DD` の文字列か、0:00 にそろえた `time.Time` で行う
- 「週の始まり」は、今日から曜日の数（日曜 = 0）だけ戻した日の 0:00（ローカル）。曜日の番号は Go の `time.Weekday` と同じ（日 = 0 … 土 = 6）で、`checkbox_property_name` の 7 列の並び（日〜土）と一致する
- 日付の足し引き（「今日＋8 日」「週の始まり」「52 週前」など）は、`time.Date(年, 月, 日 ± n, 0, 0, 0, 0, time.Local)` か `AddDate` の暦の計算で行う。時間の長さ（`24 * time.Hour` の倍数）で足し引きしない（夏時間のある TZ で 1 時間ずれるため）

#### 1.2 Notion の日付の読み方

- Date プロパティは `start` と `end`（null 可）を持つ。使う値は `end` があれば `end`、無ければ `start`
- 値が 10 文字（`YYYY-MM-DD`）ならそのまま日付とする
- 時刻付きでオフセットがある値（RFC 3339。例 `2026-11-12T23:30:00.000+09:00`）は、`time.Parse` してから `time.Local` に直し、その日付を取る
- 時刻付きでオフセットが無い値（例 `2026-11-12T09:00:00.000`）は、Date プロパティの `time_zone` があればその TZ（`time.LoadLocation`）、無いか読めなければ `time.Local` の時刻として読み、`time.Local` に直して日付を取る
- 読めない値は「期日なし」として扱い、警告をログに出す

#### 1.3 ゴールの読み方（`parseGoal`）

- 対象: Select ならその名前、rich_text なら `plain_text` をつなげたもの
- 前後の空白を除き、全角数字（`０`〜`９`）を半角に直してから、`^週\s*([0-9]+)\s*回$` に当てる
- 1〜7 のときだけ目標回数 N とする。それ以外（空、形が違う、0、8 以上）は「読めない」（null）

#### 1.4 失敗の種類（`error_kind`）

| 値             | 条件                                                                 | 小窓での表示            |
| -------------- | -------------------------------------------------------------------- | ----------------------- |
| `auth`         | HTTP 401 / 403                                                       | 権限がありません        |
| `not_shared`   | HTTP 404                                                             | DB が共有されていません |
| `rate_limited` | HTTP 429（リトライ後も失敗）                                         | 混み合っています        |
| `server`       | HTTP 5xx                                                             | Notion 側のエラー       |
| `network`      | タイムアウト・接続できない                                           | 通信エラー              |
| `config`       | `ErrInvalidConfig`（プロパティ名が空・Data Source が解決できない等） | 設定を確認してください  |
| `unknown`      | 上のどれでもない                                                     | 不明なエラー            |

- `notion.APIError` は `Status int`、`Code string`（Notion のエラー応答の `code`）、`Message string` を持つ。`Error()` の文字列は v1 と同じ形（`notion error: status=... body=...`）を保ち、既存の表示を変えない

### 2. Notion の問い合わせ（N-01）

#### 2.1 ページ送り（`queryAll`）

- 応答の型に `has_more` と `next_cursor` を足す
- `page_size` を 100 にし、`has_more` が true の間は `start_cursor` に `next_cursor` を入れて続ける
- 安全のため 50 ページ（5,000 件）で打ち切る。打ち切ったら警告をログに出し、その DB の取得結果（API-006）に `truncated: true` を入れて小窓にも出す（取れた分は使う）
- 途中のページで失敗したら、その問い合わせ全体を失敗とする（途中までの結果は使わない）

#### 2.2 タスク（期日つきの DB）

条件: `due_date_property_name`・`status_in_progress`・`status_done`・`status_paused` がすべて空でない DB。`D` は「今日＋8 日」の日付。

```json
{
  "filter": {
    "or": [
      { "property": "ステータス", "status": { "equals": "進行中" } },
      {
        "and": [
          { "property": "期限日", "date": { "on_or_before": "2026-11-20" } },
          { "property": "ステータス", "status": { "does_not_equal": "完了" } },
          { "property": "ステータス", "status": { "does_not_equal": "中断" } }
        ]
      }
    ]
  },
  "sorts": [{ "timestamp": "last_edited_time", "direction": "descending" }],
  "page_size": 100
}
```

- `status_property_type` が `select` のときは、`"status"` のキーを `"select"` にする（`equals` / `does_not_equal` はそのまま）
- Select 型でステータスが空のページが `does_not_equal` に当たらない場合、そのページ（ステータス未設定で期日のあるタスク）は表示されない。`is_empty` を足すと 3 段になり Notion の上限（2 段）を超えるので、この取りこぼしは受容する（2026-11-12。今のタスク DB は Status 型で、空にならないので影響しない）。実装時に Select 型の DB を作って挙動を確かめ、結果を Notes に残す
- 期間の期日にこの条件が開始日で効いても、開始日 ≦ 終了日なので取りこぼしは起きない（多めに取れた分は 3 章の判定で落とす）
- 条件がそろわない DB は v1 と同じ問い合わせ（進行中のみ、`page_size = max_results`、ページ送りなし）にして、v1 と同じ結果を保つ

#### 2.3 習慣（曜日の列が 7 つの DB）

今週分（毎回のポーリング）:

```json
{
  "filter": {
    "timestamp": "created_time",
    "created_time": { "on_or_after": "2026-11-08T00:00:00+09:00" }
  },
  "sorts": [{ "timestamp": "created_time", "direction": "descending" }],
  "page_size": 100
}
```

過去分（起動・日付の切り替わり・手動更新・設定の保存のとき）:

```json
{
  "filter": {
    "and": [
      {
        "timestamp": "created_time",
        "created_time": { "on_or_after": "2025-11-09T00:00:00+09:00" }
      },
      {
        "timestamp": "created_time",
        "created_time": { "before": "2026-11-08T00:00:00+09:00" }
      }
    ]
  },
  "sorts": [{ "timestamp": "created_time", "direction": "descending" }],
  "page_size": 100
}
```

- 日時は「週の始まり」をローカル時刻のオフセット付き RFC 3339 で渡す。過去分の始まりは、今週の始まりから暦で 364 日前（1.1 の計算）
- `checkbox_property_name` が 7 列でない DB は v1 と同じ問い合わせ（フィルタなし、`page_size = max_results`）で、v1 と同じ処理（同名の最新 1 件、未チェックのみ）にする。週集計はしない

#### 2.4 ページから読む値（`mapTasks` の置き換え）

- 読むプロパティ: タイトル、ステータス、今日の列、期日、優先度、ゴール、7 列（引数の形は実装者が決める）
- タスクには `due_date`（1.2 で求めた日付、無ければ空）、`priority`（Select の名前、無ければ空）を入れる
- 習慣ページは、内部の型（習慣名＝タイトルの前後の空白を除いたもの、ID、URL、作成日時、7 列の真偽値、ゴールの文字列）にする
- ページの型に `created_time` を足す

### 3. タスクの振り分けと並び順（`buildTaskView`、FR-01〜03）

入力: タスクの配列、DB の設定、`now`。出力: 並べ替えた配列（各要素に `group`・`is_in_progress` を入れる）。

1. `is_in_progress` = `status == status_in_progress`
2. `group` は上から順に最初に当てはまったもの
   | group         | 条件                                             |
   | ------------- | ------------------------------------------------ |
   | `overdue`     | 期日あり、かつ期日 < 今日                        |
   | `today`       | 期日 = 今日                                      |
   | `in_progress` | `is_in_progress`                                 |
   | `upcoming`    | 期日あり、かつ 今日 < 期日 ≦ 今日＋7 日          |
   | （捨てる）    | 上のどれでもない（期日が今日＋8 日の未着手など） |
3. 並び順: まとまりの順（overdue → today → in_progress → upcoming）。まとまりの中は次の順
   | group         | 並べるキー（左が先）                       |
   | ------------- | ------------------------------------------ |
   | `overdue`     | 期日の古い順 → 優先度 → 最終編集の新しい順 |
   | `today`       | 優先度 → 最終編集の新しい順                |
   | `in_progress` | 優先度 → 最終編集の新しい順                |
   | `upcoming`    | 期日の近い順 → 優先度 → 最終編集の新しい順 |
4. 優先度の順位: `priority_order` の中の位置（0 が最上位）。配列に無い値・空は `len(priority_order)`（最後）。`priority_order` が空なら全タスク同じ順位
5. v1 の問い合わせを使う DB（2.2 の条件がそろわない）は、全件を `in_progress` にし、最終編集の新しい順（v1 と同じ）
6. 件数（⚠）: 全タスク DB の `overdue` と `today` の数の合計

### 4. 習慣の週集計（`buildHabitSummary`、FR-06・07）

入力: 今週のページ、過去のページ（未取得なら「無し」）、設定、`now`。

1. **今週の行**: 今週のページを習慣名でまとめ、作成日時が最も新しい 1 件を使う。行の並びは、その 1 件の作成日時の新しい順（v1 の一覧と同じ）
2. **今週の値**: `days`（7 列）、`today_index`（今日の曜日）、`checked_count`（`days` の true の数）、`goal`（1.3。読めなければ null）
3. **連続達成週数**（`goal` が null、または過去分が一度も取れていなければ `streak = null`）
   1. 過去のページを（習慣名、そのページの週の始まり）でまとめ、作成日時が最も新しい 1 件を使う
   2. k = 1（先週）から 52 まで遡る。その週のページが無ければ飛ばす。ページのゴールが読めなければ飛ばす。チェック数 ≧ そのページの N なら `streak` を 1 増やす。届いていなければそこで止める
   3. 一度も止まらず、かつ k = 52 の週にその習慣のページがあれば `streak_capped = true`（このとき画面は `streak` の値によらず「52+」と出す）
   4. 今週の `checked_count` ≧ `goal` なら、`streak` に 1 を足す
4. **危ない習慣**（`risk`）: `D = 7 − today_index`、`R = goal − checked_count`
   | 条件（上から順に判定）                                             | risk          |
   | ------------------------------------------------------------------ | ------------- |
   | `goal` が null、または `days[today_index]` が true、または `R ≦ 0` | 空            |
   | `R == D`                                                           | `at_risk`     |
   | `R > D`                                                            | `unreachable` |
   | それ以外                                                           | 空            |
5. **今日の一覧**（API-002）: 今週の行のうち `days[today_index]` が false のもの。v1 と同じく `dto.Task` で返し、`risk` を入れる
6. **件数（✓）**: 全習慣 DB の、今週の行のうち「`days[today_index]` が false で、`goal` が null または `checked_count < goal`」の数の合計。7 列でない（v1 の処理の）習慣 DB は数えない
7. 今週のページが 1 件も無いとき: 今日の一覧は空、週集計は `rows: []`、`reason: "no_week_pages"`。今週分の取得が一度も成功していないときは `reason: "not_loaded"`

### 5. 取得の流れと状態

#### 5.1 ポーリング（`refreshAll`、`refreshMu` の中）

1. 有効な DB を v1 と同じく直列に処理する。タスク DB は 2.2、習慣 DB は 2.3 の「今週分」（7 列でなければ v1）
2. 成功したらキャッシュを差し替え、その DB の取得結果を `ok`・時刻ありにする。失敗したらキャッシュは残し、取得結果に失敗の種類と時刻を入れ、DB 名と種類をログに出す
3. 全 DB が終わったら `ready = true`（初回のみ変わる）
4. ローカルの日付が前回と違えば、過去分の取得を要求する（5.2）。過去分が予約済みで再試行の時刻を過ぎていれば、それも要求する
5. `refreshMu` を外してから、件数の合図を送る（5.3）

手動更新・操作後の再取得（`GetTasks` / `GetHabits` の `force_refresh = true`）も 2 と 5 を行う。過去分を「強制」で要求するのは、`GetHabits` に `refresh_history = true` が来たとき（手動更新のときだけ）。操作後の再取得では過去分を取らない。

手動更新とは次の 2 つで、どちらも `refresh_history: true` を付ける。習慣のチェックや「完了」「開始」などの後の再取得（`app.js` の `updateTaskStatus` / `updateHabitCheck` から呼ぶもの）には付けない。
- 各タブの更新ボタン（`app.js` の `refreshBtn`）
- トレイメニューの「リフレッシュ」（`main.go` の `refresh` イベント → `refreshActiveView(true)`）

#### 5.2 過去分の取得（`historyMu`、別の goroutine）

状態: `fetched_for`（取った日付）、`running`、`pending`、`pending_force`、`next_retry_at`、`retry_step`（0〜2）、DB ごとの最後の失敗。

- 要求（`requestHistory(force)`）
  - `running` なら `pending = true` にし、強制の要求なら `pending_force = true` にもして終わる
  - 強制でないとき、`fetched_for == 今日` か、`now < next_retry_at` なら何もしない
  - それ以外は `running = true` にして goroutine を起こす
- goroutine: 7 列の習慣 DB を順に 2.3 の「過去分」で取る
  - DB ごとに、成功したらキャッシュを丸ごと差し替える。失敗したら前のキャッシュを残し、失敗を記録する
  - 全 DB が成功したら `fetched_for = 今日`、`retry_step = 0`、`next_retry_at` を消す
  - 1 つでも失敗したら `next_retry_at = now + [5 分, 15 分, 60 分][retry_step]` とし、`retry_step` を 1 増やす（上限 2。以後は 60 分ごと）
  - 最後に `running = false`。`pending` なら、`pending_force` の値で（true なら強制で）もう一度要求し、`pending` と `pending_force` を false に戻す。件数の合図を送る
- 起動時は、最初の `refreshAll` が終わった後に要求する

#### 5.3 件数の合図とラベル

- `main.go` は容量 1 の channel（`make(chan struct{}, 1)`）を作り、起動時に 1 回 `SetSummaryNotifier` で渡す。合図はブロックしない送り方で、すでに 1 つ溜まっていれば捨てる
- `main.go` の受け取り goroutine: 合図のたびに `core.TraySummary()` を呼び、`formatTrayLabel` の結果を `systray.SetLabel` に渡す
- 受け取り goroutine は、設定の保存のあとの取り直し（5.4）が終わるまでは `SetLabel` を呼ばず、前のラベルを残す
- `TraySummary`: `ready`、`has_error`（有効な DB のうち、ポーリングの取得結果が失敗のものがある）、`due_count`（3 章 6）、`habit_count`（4 章 6）。過去分の失敗は `has_error` に含めない
- `formatTrayLabel`
  | 状態                                   | ラベル                                                                                                          |
  | -------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
  | トークンが無い、または有効な DB が無い | 空                                                                                                              |
  | `ready` が false                       | 空                                                                                                              |
  | `has_error`                            | `!`                                                                                                             |
  | それ以外                               | `⚠{due_count}` と `✓{habit_count}` のうち 0 でないものを半角スペースでつなぐ（例 `⚠3 ✓2`、`✓1`、両方 0 なら空） |
- 起動の順番: `NewApp` → `LoadConfig` → `application.New` → `setupTray` → 受け取り goroutine を起動 → `SetSummaryNotifier` → `StartBackgroundPolling`（今の `main.go:93` から移す）

#### 5.4 設定の保存（API-008 の後）

- 設定の世代番号（`generation`）を 1 増やし、`reloading = true` にする
- 全 DB のタスク・今週分・過去分のキャッシュと取得結果を捨てる。`ready` は残す
- ポーリングをやり直し（v1 と同じ）、過去分を「強制」で要求する
- 取り直しの最初の `refreshAll` が終わったら `reloading = false` にして、件数の合図を送る（それまでラベルは前の値のまま）

#### 5.5 キャッシュの排他と世代
- キャッシュ（タスク、今週分、過去分）、取得結果、`ready` / `reloading` / `generation` は、既存の `cacheMu` で守る。`refreshMu` と `historyMu` は「取得を同時に 2 本走らせない」ためのロックで、キャッシュは守らない
- 取得（`refreshAll`、強制取得、過去分の goroutine）は、始める時の `generation` を覚え、キャッシュに書く直前に `cacheMu` の中で今の `generation` と比べる。違えば（途中で設定が保存されたら）結果を捨てて書かない
- 読む側（`GetTasks` / `GetHabits` / `getHabitSummary` / `TraySummary` / `getSyncState`）は、`cacheMu` の中で必要なものを写し取り、ロックを外してから計算する
- 件数の合図は、`cacheMu` も `refreshMu` も外してから送る

### 6. RPC の詳細

共通: 要求は `{id, action, payload}`、応答は `{id, ok, data, error}`（v1 のまま）。`ok: false` のとき `error` は文字列。`database_key` は設定にある DB のキーでなければならず、無ければ `database not found`、種類が違えば `database kind is not task`（または `habit`）を返す。

#### API-001 `getTasks`（変更）

- 要求: `{ "database_key": "tasks", "force_refresh": false }`
  - `database_key`: 必須、空でない文字列、task の DB
  - `force_refresh`: 省略時 false。true なら 5.1 の 2 と 5 を行ってから返す。false ならキャッシュを使い、無ければ取得する
- 応答 `data`（3 章で並べ替え済み）:

```json
[
  {
    "id": "abc",
    "title": "請求書を出す",
    "url": "https://www.notion.so/...",
    "status": "待ち",
    "last_edited_time": "2026-11-10T03:00:00.000Z",
    "checked": false,
    "due_date": "2026-11-10",
    "priority": "高",
    "group": "overdue",
    "is_in_progress": false
  }
]
```

- 失敗の返し方（NFR-04 に合わせる）
  - Notion からの取得に失敗しても、その DB のキャッシュがあれば `ok: true` でキャッシュを返す。失敗は取得結果（API-006）に載り、小窓の共通部（SC-003）に出る
  - キャッシュが無いときだけ `ok: false` にする。`error` はトークン未設定 `token is not set`、設定不備（`ErrInvalidConfig` の文言）、Notion の失敗（v1 と同じ文言）

#### API-002 `getHabits`（変更）

- 要求: API-001 の形に `refresh_history`（省略時 false）を足したもの（habit の DB）。`refresh_history = true` なら過去分を強制で要求する（5.1）
- 失敗の返し方は API-001 と同じ（キャッシュがあれば `ok: true`）
- 応答 `data`: v1 と同じ `Task` の配列（4 章 5）。各要素に `risk`（`at_risk` / `unreachable` / 空）を足す

#### API-003 `getHabitSummary`（新規）

- 要求: `{ "database_key": "habits" }`。キャッシュだけを読み、取得はしない
- 応答 `data`:

```json
{
  "enabled": true,
  "reason": "",
  "today_index": 3,
  "history_loading": false,
  "rows": [
    {
      "name": "ストレッチ",
      "url": "https://www.notion.so/...",
      "days": [true, false, true, false, false, false, false],
      "checked_count": 2,
      "goal": 5,
      "streak": 3,
      "streak_capped": false,
      "risk": "at_risk"
    }
  ]
}
```

- `enabled: false` のとき `reason` は `weekday_columns`（7 列でない）。`enabled: true` で今週のページが無いときは `reason: "no_week_pages"`、今週分の取得が一度も成功していないときは `reason: "not_loaded"`。どちらも `rows: []`。この RPC はキャッシュだけを読むので、DB が見つからない・種類が違うとき以外は `ok: false` にならない
- `history_loading`: 過去分を取っている最中で、一度も取れていないとき true（連続達成週数は null）

#### API-006 `getSyncState`（新規）

- 要求: `{}`（payload なし）
- 応答 `data`: 有効な DB ごとの配列

```json
[
  {
    "database_key": "tasks",
    "name": "タスク",
    "ok": false,
    "last_attempt_at": "2026-11-12T09:01:00+09:00",
    "last_success_at": "2026-11-12T09:00:00+09:00",
    "error_kind": "network",
    "history_ok": true,
    "history_fetched_for": "",
    "history_error_kind": "",
    "truncated": false
  }
]
```

- 習慣以外の DB と 7 列でない習慣 DB では、`history_ok` は null、ほかの `history_*` は空。過去分を一度も試していない習慣 DB も `history_ok: null`
- 一度も試していなければ `last_attempt_at` は空
- `truncated`: 直近の取得で 50 ページの上限で打ち切ったら true（2.1）

#### API-008 `saveConfig`（変更）

- 要求: v1 の設定に次のキーを足したもの。保存の前に検証する
  | キー                     | 検証                                                                   |
  | ------------------------ | ---------------------------------------------------------------------- |
  | `due_date_property_name` | 文字列。前後の空白を除く。空可                                         |
  | `priority_property_name` | 文字列。前後の空白を除く。空可                                         |
  | `priority_order`         | 文字列の配列。各要素の前後の空白を除き、空の要素と重複を除く。空配列可 |
  | `goal_property_name`     | 文字列。前後の空白を除く。空可                                         |
- habit の DB に task 用のキー、task の DB に habit 用のキーが入っていても、エラーにせずそのまま保存し、使わない
- `priority_property_name` が空なら、`priority_order` は空配列にして保存する（画面に頼らずサーバ側でそろえる）
- 保存後に 5.4 を行う

API-004 `updateStatus`・API-005 `updateHabitCheck`・API-007 `getDatabaseProperties` は v1 のまま（4 は `action: "resume"` を「開始」に使う）。

### 7. 画面の詳細

#### SC-001 タスクタブ

- 初回の表示: 最初の `getTasks` の応答が来るまでは「読み込み中」を出す（v1 の空の表示と区別する）
- 構成（上から）: まとまりごとに見出し「期限切れ（n）」「今日（n）」「進行中（n）」「直近 7 日（n）」と、その下にタスクの行。0 件のまとまりは見出しごと出さない。全部 0 件なら「タスクがありません」。期日を使わない DB は、見出しなしで並べる
- 「直近 7 日」は見出しを押すと開閉する。既定は閉じる。状態は `localStorage` の `nudge.upcomingOpen.<database_key>` に `"1"` / `"0"` で保存し、読み書きは try/catch で囲む（失敗したら閉じた状態）
- 行の中身（1 件 1 行）
  - 左: 完了の丸（完了の値が設定されているときだけ）。押すと完了にする
  - 中: タイトル（2 行まで。押すと Notion で開く）と、メタ行（期日・優先度・「更新 HH:MM」）。期日は `overdue` なら「11/10（2 日超過）」、`today` なら「今日」、ほかは「11/14」
  - 右: 未着手（`is_in_progress` が false）なら小さな「開始」。⋯ を押すと、行の下に「Notionで開く」と、進行中なら「中断」を出す（小窓の端で切れないよう、浮かせずに行の中に出す）
- 強調: `overdue` の行に `is-overdue` を付け、左の線と期日を赤系にする
- 「開始」: `updateStatus`（`action: "resume"`）→ 成功したら `force_refresh: true` で取り直す。失敗したら `setError` に出し、一覧は変えない
- 日付の切り替わり: 再描画は既存の定期処理（`setInterval`）で行われ、振り分けは Go 側で毎回計算されるので、JS 側で日付を見る処理は足さない

#### SC-002 習慣タブ

- 1 習慣 = 1 行（今日のチェックと今週の状況を統合。ADR-20261112-0016）
  - 1 段目: 今日のチェック（丸い枠。乗せると ✓ が薄く出て、押すと塗りつぶし）、習慣名（押すと Notion で開く）、`risk = at_risk` なら「今日やらないと届かない」のラベル、回数（「2/5」、`goal` が null なら「2」）、連続達成週数（「連続 3 週」、`streak_capped` なら「連続 52+ 週」。0 週と null は出さない。過去分の取得中は「…」）
  - 2 段目: 今週の 7 日を同じ大きさの点で出す。済みは青の塗り、過ぎてやらなかった日は薄い枠、今日まだは橙の輪、これからの日は薄い塗り。今日の点の下に橙の短い線を引く。点には「水 済み」のようなツールチップを付ける
  - `risk = unreachable` は文字を出さず、回数を薄くするだけにする
- 並び: 今日がまだで今週の目標に届いていない習慣を上に。今日チェック済みと、今週の目標に届いた習慣は「済み（n）」にまとめて畳む（開閉は `localStorage` の `nudge.habitDoneOpen.<database_key>` に保存）。上が 0 件なら「今日の習慣はすべて済みました」
- 出し分け: `enabled: false`（曜日の列が 7 つでない）なら、v1 の今日の一覧をそのまま出す。`reason: not_loaded` なら「読み込み中…」、`no_week_pages` なら「今週のページがまだありません」
- 呼ぶ順: `getHabits`（今日の一覧。チェックに使う ID を得る）のあとに `getHabitSummary` を呼ぶ。`history_loading` の間は 3 秒ごとに `getHabitSummary` を読み直す（最大 20 回）。`getHabitSummary` が失敗したら v1 の一覧を出し、`setError` に出す

#### SC-003 小窓の共通部

- ヘッダーは 1 行（ロゴ・「Nudge」・接続の点・更新のアイコン・設定のアイコン）。接続できているときは緑の点だけを出し、文字はツールチップにする。タブ名と重なるペインの見出しは出さない
- 更新のアイコンは手動更新として扱い、`refresh_history: true` を付ける（習慣の過去の週も取り直す）
- フッター（`lastUpdated` / `errorText`）の上に、取得状態の 1 行を出す
- `getSyncState` を、既存の定期処理と表示切り替えのたびに呼ぶ
- `ok: false` の DB があれば「取得に失敗: {name}（{1.4 の表示}） HH:MM」。複数あれば最初の 1 件と「ほか n 件」
- 過去分だけの失敗は「過去の習慣の取得に失敗（連続週数は前回の値）」。`truncated` の DB があれば「{name} の件数が多すぎて一部を省きました」。すべて成功なら何も出さない

#### SC-004 設定ウィンドウ

- 構成: 左に「一般」と DB の一覧（名前の下に種類と「無効」かどうか）と「＋ データベースを追加」。右に選んだものの設定を 1 つずつ出す
- 一般: Notion API トークン（保存済みなら「変更」で入れ直す。「トークンを削除」は確認してから消す）と「ログイン時に起動する」
- DB の設定は節に分ける: 「タブ（名前・種類・小窓にタブを出す）」「Notion のデータベース（データベース ID と『読み込む』）」「ステータス」「期日と優先度」（習慣は「曜日の列とゴール」）。自動で埋まる欄（データソース ID・タイトルのプロパティ・ステータスの種類）は「詳細」に畳む
- 設定画面を開いたとき、データベース ID のある DB はプロパティを読み込んでおく。読み込みの結果は「✓ 読み込み済み（プロパティ n 件、時刻）」、失敗したら理由を出す
- 期日（`type: date`）・優先度（`type: select`）・ゴール（`type: select` と `rich_text`）は、読み込んだプロパティから選ぶ。候補が 0 件なら「該当する型のプロパティがありません」と添える
- 優先度の順: 優先度のプロパティを選ぶと、その選択肢を縦に並べて ↑ ↓ で並べ替える（上ほど高い）。初期値は保存済みの `priority_order`、無ければ Notion の選択肢の順。Notion に無い値は表示の時点で取り除き、順に無い選択肢は末尾に足す。優先度のプロパティを空にしたら、順は隠して空配列で保存する
- 保存: 右下に固定した「保存」ボタン 1 つで、トークン（入れ直したときだけ）と設定をまとめて保存する。変更があると「未保存の変更があります」（ボタンを橙に光らせる）、保存できたら「✓ 保存しました（時刻）」、失敗したら赤で理由を出す
- 保存したら、小窓にも設定を読み直させる（`config-updated` イベント）

#### SC-005 メニューバー

- 5.3 のラベル。表示例: `⚠3 ✓2` / `⚠1` / `✓4` / `!` / 空
- メニュー（右クリック）は v1 のまま

### 8. テスト設計

`docs/evaluation/` はこのリポジトリに無いので、評価セットとの突合は対象外。

#### 単体テスト（Go）

| 対象                           | 観点                                                                                                                                                                                                                                                                                           |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| フィルタの組み立て（2.2・2.3） | status 型と select 型、期日つきと v1、今週分と過去分の JSON を期待値と比べる。オフセット付きの日時が `+09:00` で出る                                                                                                                                                                           |
| ページ送り（2.1）              | `httptest` で 2 ページ（`has_more` true → false）。ちょうど 100 件。途中で失敗したら全体が失敗。20 ページで打ち切り                                                                                                                                                                            |
| 日付の読み方（1.2）            | 日付のみ、`+09:00` 付き、UTC で日本時間だと翌日になる値、期間（終了日を使う）、読めない値                                                                                                                                                                                                      |
| ゴールの読み方（1.3）          | 「週１回」「週1回」「週 3 回」「毎日」「週0回」「週8回」、空                                                                                                                                                                                                                                   |
| 失敗の分類（1.4）              | 401・403・404・429・500・タイムアウト・`ErrInvalidConfig`                                                                                                                                                                                                                                      |
| `buildTaskView`（3 章）        | 昨日・今日・明日・今日＋7 日・今日＋8 日の境界、未着手と進行中、まとまりの順、並びのキー、`priority_order` に無い値、v1 の DB                                                                                                                                                                  |
| `buildHabitSummary`（4 章）    | 週の境界（土曜 23:59 と日曜 0:00、TZ を Asia/Tokyo と America/Los_Angeles で固定）、同じ週の同名ページ、ページの無い週を飛ばす、ゴールが読めない週を飛ばす、途中で止まる、52 週で `capped`（52 週前にページがある・無い）、今週の達成で +1、過去分が未取得なら null、危ない習慣の 4 条件、件数 |
| `formatTrayLabel`（5.3）       | 5.3 の表の全行                                                                                                                                                                                                                                                                                 |
| 過去分の取得（5.2）            | 同時に 1 本、`pending` で 1 回だけやり直す、同じ日は取らない、強制なら取る、再試行の間隔（5→15→60→60 分）、失敗で前のキャッシュが残る（時計は注入）                                                                                                                                            |
| 設定の互換（NFR-02）           | v1 の `config.json` を読み、新しいキーが空で、タスクは進行中のみ・習慣は v1 の処理になる。新しいキーを保存して読み直すと同じ値                                                                                                                                                                 |

| API の呼び出し数（NFR-01） | `httptest` で数える。平常時のポーリング 1 分で 60 件以下。過去分を取り直す 1 分も 60 件以下。習慣のチェックの後に過去分の問い合わせが出ない |
| 設定の保存（FR-08・API-008） | 前後の空白の除去、`priority_order` の空要素・重複の除去、優先度のプロパティが空なら `priority_order` が空、種類違いのキーをそのまま保存 |
| 失敗時の表示（NFR-04・06） | 取得を失敗させると、キャッシュがあれば `ok: true` で前の結果、`TraySummary.has_error` が true でラベルが `!`、API-006 に失敗の種類、ログに DB 名と種類 |
| 世代と競合（5.4・5.5） | 過去分の取得中・`refreshAll` 中に設定を保存すると、古い世代の結果が書かれない。保存の後、取り直しが終わるまでラベルが変わらない |
| 強制要求の畳み込み（5.2） | 実行中に強制の要求が来たら、終わった後に強制で 1 回だけ取り直す |
| 日付の読み方の追加（1.2） | オフセット無しで `time_zone` あり・なし。夏時間の切り替え週（America/Los_Angeles）で週の始まりと「今日＋8 日」がずれない |

#### 結合テスト

- `internal/app/app_flow_test.go` の `TestTaskAndHabitFlow` は、習慣の設定が 7 列かどうかで経路が分かれる。v1 の経路（1 列）を残したまま、7 列の経路（今週分・過去分・週集計）のテストを別に足す
- 件数の合図: ポーリング・強制取得・設定の保存のあとに合図が届き、`TraySummary` が期待値になる

#### 手動テスト

- 実機でメニューバーのラベルが出る・消える・「!」になる（ネットワークを切る）
- 「開始」を押したとき、Notion のオートメーションで作業時間の記録が始まるか（要件の確認事項）
- 0:00 をまたいだあと、次のポーリングで振り分けとラベルが切り替わる
- 日曜未明、今週のページが作られる前の習慣タブの表示
