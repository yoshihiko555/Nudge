# Nudge

Notion のタスクと習慣を macOS メニューバーで確認・更新する Wails v3 アプリです。ローカル完結で動作します。

## 主な機能
- タスクの一覧表示（期限切れ・今日・進行中・直近 7 日のまとまり、期日と優先度の順）
- 完了 / 中断 / 開始（未着手 → 進行中）のステータス更新
- 習慣の今日のチェックと、今週 7 日の状況・連続達成週数（「週N回」のゴール）の表示
- メニューバーの件数（期限切れ＋今日のタスク、今日残っている習慣。取得に失敗したら「!」）
- 手動更新と自動ポーリング
- Notion 設定の UI からの保存

## 前提
- Notion のタスクは Database で管理されている
- Integration を作成し、対象 Database を共有済み

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
4. 設定画面（メニューバーの「設定」）で以下を保存
   - 一般: Notion API トークン
   - データベースごと: データベース ID（「読み込む」でプロパティの候補が出る）、ステータスのプロパティと進行中 / 完了 / 中断 の値
   - タスク（任意）: 期日のプロパティ（Date）、優先度のプロパティ（Select）と優先度の順
   - 習慣（任意）: 曜日のチェックの列（日〜土）、ゴールのプロパティ（値は「週N回」）
   - データソース ID・タイトルのプロパティ・ステータスの種類は「詳細」にあり、空なら自動で入る
   - 期日・優先度・ゴールが空なら、v1 と同じ「進行中のみ」「今日の習慣のみ」の表示になる

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
        "checkbox_property_name": "",
        "due_date_property_name": "期限日",
        "priority_property_name": "優先度",
        "priority_order": ["高", "中", "低"],
        "goal_property_name": ""
      }
    ],
    "poll_interval_seconds": 60,
    "max_results": 30
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
