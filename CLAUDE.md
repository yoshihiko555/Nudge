# Nudge

**概要**: Notion の「進行中タスク」を macOS メニューバーで確認・更新する Wails v3 アプリ。ローカル完結で動作。

---

## Tech Stack

- **Language**: Go 1.24.0
- **Framework**: Wails v3 (v3.0.0-alpha.53)
- **Build Tool**: go-task (`task`)
- **Package Manager**: go modules

---

## Commands

```bash
# 依存関係インストール
go mod download

# 開発サーバー起動（Vite ポート 9245）
task dev

# 現在の OS 向けに実行
task run

# 現在の OS 向けにビルド
task build

# テスト実行
go test ./...

# 配布用パッケージ作成
task package

# DMG 作成（macOS）
task dmg VERSION=x.x.x
```

---

## Project Structure

```
cmd/nudge/           # エントリーポイント / 埋め込み UI 資産
internal/
├── app/             # アプリのユースケースと制御
├── dto/             # データ転送オブジェクト
├── log/             # ロギング
├── notion/          # Notion API クライアント
├── store/           # 設定ファイル / Keychain 永続化
└── sync/            # ポーリング処理
build/               # Wails のビルド/配布設定
docs/                # 設計資料・ADR
```

---

## Coding Conventions

- コミット形式: `<type>(<scope>): <summary>`（例: `feat(tray): add refresh menu action`）
- Type: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`, `perf`, `style`, `revert`
- 命名: Go 標準（PascalCase for exported, camelCase for unexported）
- 設定ファイル: `snake_case` + 2 スペースインデント

---

## Security

- Notion API トークンは macOS Keychain に保存（Service: `nudge-notion`, Account: `notion-api-token`）
- トークンをリポジトリにコミットしない
- 設定ファイル: `~/Library/Application Support/Nudge/config.json`

---

## Docs

- `docs/adr/`: Architecture Decision Records（設計決定の履歴）
- `docs/basic-design.md`: 基本設計
- `docs/detail-design.md`: 詳細設計
- `docs/commit-conventions.md`: コミット規約

---

## Notes

- Notion Integration を作成し、対象 Database を共有済みであることが前提
- macOS 専用（Wails v3 によるメニューバーアプリ）
