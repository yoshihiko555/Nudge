# ADR-0009: Brain メモをテンプレート起点で新規作成する

## 決定

1. Brain メモはメイン/設定とは別ウィンドウ（`?mode=brain`）で作成する
2. Brain は Notion のテンプレートページを参照し、タイトル・アイコンはテンプレート由来を採用する
3. ユーザー入力は本文のみとし、登録時に Brain データベースへ新規ページを作成する
4. 設定で Brain Database ID / Brain Template Page ID を管理する

## 理由

- 既存のタスク/習慣 UI と分離し、思考メモの入力を最短導線にする
- テンプレート起点にすることでタイトル/アイコン/構成の統一を保てる
- 入力項目を最小化し、素早い思考記録に寄せる

## 代替案

- Brain を通常のDBタブに統合 → UI/操作が混雑するため不採用
- タイトル/プロパティをアプリで入力 → 入力負荷が高くテンプレ運用と相性が悪いため不採用
- テンプレートのクローンAPIを使用 → Notion API制約があり実装リスクが高いため不採用

## 影響

- `cmd/nudge/main.go`: Brain ウィンドウ追加 / RPC 追加
- `internal/notion/brain.go`: テンプレート取得 / 新規ページ作成
- `cmd/nudge/assets/index.html`: Brain 画面と設定項目の追加
- `cmd/nudge/assets/app.js`: Brain 画面の初期化・登録処理
