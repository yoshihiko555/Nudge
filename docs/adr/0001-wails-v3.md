# ADR-0001: Wails v3 を採用する

## 決定
Wails v3 を採用する。

## 理由
- System Tray 機能とウィンドウ attach の公式サポートがあり、メニューバー常駐要件に合致
- Go で完結し、軽量に実装できる

## 代替案
- Wails v2 + 外部ライブラリ
- Electron

## 影響
- API 詳細は公式ドキュメントに準拠して実装する（System Tray / Window attach）
