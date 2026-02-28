# ADR-0009: Brain メモをテンプレート起点で新規作成する

## 状態

廃止（2026-02-28）

## 旧決定（履歴）

1. Brain メモはメイン/設定とは別ウィンドウ（`?mode=brain`）で作成する
2. Brain は Notion のテンプレートページを参照し、タイトル・アイコンはテンプレート由来を採用する
3. ユーザー入力は本文のみとし、登録時に Brain データベースへ新規ページを作成する
4. 設定で Brain Database ID / Brain Template Page ID を管理する

## 廃止理由

- 現行要件が task / habit の運用に集約され、Brain 専用フローを維持する必要がなくなった
- 設定項目と UI 動線を簡素化し、既存機能の保守性を優先する

## 廃止に伴う影響

- `cmd/nudge/main.go`: Brain ウィンドウ/RPC 分岐を削除
- `internal/notion/brain.go`: Brain 専用 Notion API 実装を削除
- `cmd/nudge/assets/index.html`: Brain 画面と設定項目を削除
- `cmd/nudge/assets/app.js`: Brain 画面の状態管理・登録処理を削除
