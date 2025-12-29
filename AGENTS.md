# Repository Guidelines

## プロジェクト概要
Notion の進行中タスクを macOS メニューバーで確認・更新する Wails v3 アプリ。ローカル完結で動作します。

## Project Structure & Module Organization
- `cmd/nudge/main.go`: エントリーポイント。`cmd/nudge/assets/` は埋め込み UI 資産。
- `internal/app`: アプリのユースケースと制御。
- `internal/notion`: Notion API クライアント。
- `internal/store`: 設定ファイルと macOS Keychain トークンの永続化。
- `internal/sync`, `internal/tray`, `internal/dto`, `internal/log`: 同期・UI・DTO・ログ。
- `build/`: Wails のビルド/配布設定。`docs/` は設計資料。`bin/` は成果物。

## Build, Test, and Development Commands
- `task dev`: Wails 開発モード（デフォルト Vite ポートは 9245）。
- `task run`: 現在の OS 向けに実行。
- `task build`: 現在の OS 向けにビルド。
- `task package`: 配布用パッケージ作成。
- `task setup:docker`: クロスコンパイル用 Docker イメージ作成。
- 事前に `task`（go-task）と `wails3` を用意してください。

## Coding Style & Naming Conventions
- Go は `gofmt` 準拠。公開は `CamelCase`、非公開は `lowerCamel`.
- JSON 設定は `snake_case` と 2 スペース（`config.example.json`）。

## Testing Guidelines
- 現状 `*_test.go` は未配置。追加時は同一パッケージに配置。
- 実行コマンド: `go test ./...`.

## Commit & Pull Request Guidelines
- コミット規約は `docs/commit-conventions.md` を参照（Conventional Commits 互換）。
- 例: `feat(tray): add refresh menu action`, `docs: add commit conventions`.
- PR には目的、変更範囲、確認手順を記載。UI 変更はスクリーンショットを添付。

## Security & Configuration Tips
- 設定は `os.UserConfigDir()/Nudge/config.json` に保存。
- Notion トークンは macOS Keychain に保存。実トークンはコミット禁止。
- 新しい設定値は `config.example.json` にサンプルで追記。
