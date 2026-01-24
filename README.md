# Nudge

Notion の「進行中タスク」を macOS メニューバーで確認・更新する Wails v3 アプリです。ローカル完結で動作します。

## 主な機能
- 進行中タスクの一覧表示
- 完了 / 中断へのステータス更新
- 手動更新と自動ポーリング
- Notion 設定の UI からの保存
- Brain データベースへのメモ追加（テンプレート起点）

## 前提
- Notion のタスクは Database で管理されている
- Integration を作成し、対象 Database を共有済み
- Notion API のバージョン（`YYYY-MM-DD`）を指定する

## 動作環境・必須ツール
- macOS
- Go
- Wails v3 (`wails3`)
- go-task (`task`)

## クイックスタート
1. Wails v3 と task を用意
2. Notion Integration を作成し、Database を共有
3. アプリ起動
   ```sh
   task run
   ```
4. アプリの「設定」タブで以下を保存
   - Notion API トークン
   - Database ID
   - Data Source ID（ボタンで自動取得可）
   - Title / Status プロパティ名
   - Status 型（`status` / `select`）
   - 進行中 / 完了 / 中断 の値
   - Brain Database ID
   - Brain Template Page ID
   - Notion Version（`YYYY-MM-DD`）

## 設定ファイル
- 保存先: `~/Library/Application Support/Nudge/config.json`
- 形式は `snake_case` + 2 スペースインデント
- 例:
  ```json
  {
    "databases": [
      {
        "key": "tasks",
        "name": "タスク",
        "kind": "task",
        "enabled": true,
        "database_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        "data_source_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        "title_property_name": "Name",
        "status_property_name": "Status",
        "status_property_type": "status",
        "status_in_progress": "In Progress",
        "status_done": "Done",
        "status_paused": "Paused",
        "checkbox_property_name": ""
      }
    ],
    "poll_interval_seconds": 60,
    "max_results": 30,
    "notion_version": "YYYY-MM-DD",
    "brain_database_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    "brain_template_page_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
  }
  ```

## セキュリティ
- Notion API トークンは macOS Keychain（Service: `nudge-notion`, Account: `notion-api-token`）に保存
- リポジトリへのトークンのコミットは禁止

## 開発コマンド
```sh
# 開発モード（デフォルト Vite ポート 9245）
task dev

# task dev が動かない場合
wails3 task run dev

# 現在の OS 向けに実行
task run

# 現在の OS 向けにビルド
task build

# 配布用パッケージ作成
task package

# クロスコンパイル用 Docker イメージ作成
task setup:docker
```

## テスト
```sh
go test ./...
```

## ディレクトリ構成（抜粋）
- `cmd/nudge`: エントリーポイント / 埋め込み UI 資産
- `internal/app`: アプリのユースケースと制御
- `internal/notion`: Notion API クライアント
- `internal/store`: 設定ファイル / Keychain 永続化
- `internal/sync`, `internal/tray`, `internal/dto`, `internal/log`
- `build/`: Wails のビルド/配布設定
- `docs/`: 設計資料
