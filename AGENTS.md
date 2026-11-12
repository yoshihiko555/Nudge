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
- `task --list`: タスクの一覧と説明を出す。
- `task run`: ビルドしてから起動する（`task build` を含む）。
- `task dev`: Wails 開発モード。ファイルを変えると、ビルドし直して起動し直す。
- `task build`: ビルドだけ行う（`bin/Nudge`）。
- `task test`: `go test ./...` を実行する。
- `task package`: 配布用の `.app` を作る。`task dmg VERSION=x.y.z` / `task dmg:sha256 VERSION=x.y.z` で DMG と SHA256。
- 事前に `task`（go-task）を用意してください。`wails3` は `go run` で go.mod と同じ版を使うので、別に入れなくてよい。

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
