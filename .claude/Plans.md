# Plans

## Project: Nudge 汎用化

### Phase 1: デフォルト設定の汎用化 `cc:done`

#### デフォルトDB構成の見直し

- `cc:done` デフォルトデータベースを空にする（初回は必ずセットアップを通す）
- `cc:done` 習慣(Habit)のハードコードされた日本語デフォルト値を除去（`"名前"`, `"日,月,火,水,木,金,土"`）

#### レガシーコード削除

- `cc:done` `brain_database_id` / `brain_template_page_id` のマイグレーション処理を削除
- `cc:done` 旧フラット形式(`database_id` 等)の config マイグレーション処理を削除

### Phase 2: セットアップウィザード改善 `cc:TODO`

#### DB接続の自動解決

- `cc:TODO` database_id 入力後にプロパティ一覧を自動取得し、UI で選択可能にする
- `cc:TODO` title / status プロパティの自動検出・サジェスト
- `cc:TODO` status の選択肢（進行中/完了/中断）をドロップダウンで選べるようにする

#### 習慣DBのセットアップ

- `cc:TODO` 習慣DBのチェックボックスプロパティ名をユーザーが設定できるようにする

### Phase 3: UI文字列の整理 `cc:TODO`

#### ハードコード文字列の外部化

- `cc:TODO` トレイメニューの日本語文字列（"設定", "更新", "終了"）を定数化
- `cc:TODO` デフォルト名（"タスク", "習慣"）の扱いを見直す

---

## Decisions

- 2026-03-07: 配布を見据えて Notion 環境依存の設計を汎用化する

## Notes

- 現状 `defaultDatabases()` で "tasks" と "habits" の2つが固定生成される
- 習慣機能の曜日チェックボックス構造は特殊なスキーマ前提
- `FetchDatabaseProperties` / `ResolveDataSourceID` など自動解決の仕組みは既にある
