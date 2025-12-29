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
