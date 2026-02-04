# Nudge Codebase Analysis

Gemini CLI による Go コードベース分析結果（2026-02-05）

## 1. Architecture Overview

プロジェクトは **Controller-Service-Repository パターン** をデスクトップアプリ向けにアレンジした構成を採用。

### レイヤー構造

- **Entry Point (`cmd/nudge/main.go`):** Composition Root
  - 依存関係の初期化（Stores, Notion Client, Core App）
  - Wails v3 アプリケーションのセットアップ
  - Window/Tray の定義
  - RPC イベントマッピング

- **Core Domain (`internal/app`):** `App` 構造体が中心的なコントローラー
  - ビジネスロジックのオーケストレーション
  - 状態管理（タスク/習慣のキャッシング）
  - UI（Wails 経由）とバックエンドサービス（Notion, Storage）の仲介

- **Infrastructure Layers:**
  - `internal/notion`: Notion API クライアント実装
  - `internal/store`: 設定とシークレットの永続化
  - `internal/sync`: バックグラウンド処理（ポーリング）

### 通信パターン

- **Frontend → Backend:** Wails の `RawMessageHandler` を使用した JSON ベース RPC
  - `getTasks`, `updateStatus` などのアクションを `coreapp.App` にルーティング
- **Backend → Frontend:** `window.EmitEvent` で更新を UI にプッシュ

---

## 2. Wails v3 Utilization

**Wails v3 (Alpha/Beta)** API を使用。v2 とは大きく異なる。

### Application Setup

- `application.New()` + `application.MacOptions` で "Accessory" アプリとして設定
  - メニューバーのみ表示、Dock アイコンなし

### Bindings vs. Events

- 標準的な Wails メソッドバインディングは使用せず、**カスタム `RawMessageHandler`** を実装
  - JSON ペイロード（`id`, `action`, `payload`）を手動でアンマーシャル
  - 特定の関数にルーティング
  - Redux スタイルの dispatch システムを模倣

### Windows

- **Popover:** フレームレス、常に最前面の window でネイティブなメニューバードロップダウンをシミュレート
- **Settings/Brain:** 独立した標準 window
- **Hook:** `RegisterHook(events.Common.WindowClosing)` で window を破棄せず非表示にして状態を保持

### System Tray

- アクティブな設定（databases）に基づいた動的メニュー構築（`buildTrayMenu`）
- ユーザーアイコンがない場合は base64 エンコードされたフォールバックアイコン（`trayIconBase64`）を使用

---

## 3. Notion API Implementation

`internal/notion/client.go` にカスタム実装。サードパーティライブラリは不使用。

### Transport

- 標準の `net/http` クライアント、設定可能なタイムアウト（15秒）

### Resilience

- `5xx` エラーと `429 Too Many Requests` に対するリトライロジック（`WithRetry`）
- `Retry-After` ヘッダーを尊重

### Querying

- **Tasks:** `data_sources` または `databases` を特定のフィルター（ステータスチェック）でクエリ
  - 結果を汎用的な `dto.Task` にマッピング
- **Habits:** 日付でフィルターし、特定のチェックボックスプロパティをチェックする専用クエリ（`QueryHabitsToday`）

### DTOs

- 内部の `dto` パッケージを使用して、外部の Notion JSON 形式とアプリ内部のデータモデルを分離

---

## 4. Config & Security

### Configuration (`internal/store/config_store.go`)

- ユーザーの config ディレクトリ（`~/Library/Application Support/Nudge/`）に `config.json` として保存
- 複数のデータベース設定（Tasks vs. Habits）をサポート
- マイグレーションロジックを含む
  - 古いフラット設定フォーマットが見つかった場合、新しい `Databases` 配列形式にメモリ内で変換

### Secrets (`internal/store/token_store.go`)

- **実装:** CGo を使用しない（`keyring` ライブラリなど）
- 代わりに `os/exec` で macOS の `security` コマンドラインツールを直接呼び出し
  - `security find-generic-password`
- **メリット:** CGo クロスコンパイルの問題を回避
- **デメリット:** ネイティブバインディングより遅い、macOS 専用（プロジェクトの目標に合致）

---

## 5. Polling Mechanism

`internal/sync/poller.go` に実装、`internal/app/app.go` で管理。

### Concurrency

- 専用の goroutine + `time.Ticker` を使用

### Lifecycle

- `main.go` の `StartBackgroundPolling()` で設定読み込み後に開始
- 設定変更時に動的に停止/再起動可能

### Logic

- `refreshAll` を呼び出し、有効なすべてのデータベースを反復処理
- インメモリキャッシュ（`taskCache`, `habitCache`）を更新
- **注意:** poller 自体はキャッシュを更新するのみ
  - UI は明示的な `getTasks` 呼び出しや他の場所でトリガーされるイベントに依存
  - `refreshAll` は純粋に内部状態を更新

---

## 6. Improvements & Technical Debt

### 優先度: 高

1. **テストコードの不在**
   - ファイルツリー内にテストが見当たらない
   - `App` ロジックと Notion クライアントは既に依存性注入を使用しており、テスト可能
   - ユニットテスト追加が最優先の技術的負債

2. **RPC Routing の肥大化 (`main.go`)**
   - `handleRawMessage` の switch 文が巨大
   - `Router` マップや独立したハンドラーにリファクタリングすべき
   - 可読性とテスタビリティが向上

### 優先度: 中

3. **ポーリングフィードバックの欠如**
   - `poller` はキャッシュを更新するが、変更時に即座にフロントエンドにイベントを発行しない
   - UI が "pull ベース" またはユーザーの次の操作まで古い状態の可能性
   - ポーリングループに `EmitEvent("data-updated")` を追加してリアクティブにすべき

4. **Error Handling の脆弱性**
   - `token_store.go` の `security` コマンド実行が stderr の文字列解析に依存（"could not be found"）
   - macOS バージョンやロケールによって壊れやすい
   - より堅牢なエラーハンドリングが必要

### 優先度: 低

5. **ハードコードされたアセット**
   - `main.go` の `trayIconBase64` が巨大な文字列リテラル
   - 別ファイルに移動するか `embed` で処理すべき

---

## Summary

Nudge は明確なレイヤー構造を持つ、よく整理されたコードベース。Wails v3 の実験的な機能（カスタム RPC ハンドラー）を効果的に活用し、macOS ネイティブツール（`security` コマンド）を直接利用することで CGo 依存を回避。

主な改善点は、テストコードの追加、RPC ルーティングのリファクタリング、ポーリングのリアクティブ化の3点。アーキテクチャ自体は健全で、技術的負債は管理可能なレベル。
