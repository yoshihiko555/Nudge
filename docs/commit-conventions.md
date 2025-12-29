# コミット規約

## 基本形式
```
<type>(<scope>): <summary>
```
- `scope` は任意。影響範囲が明確な場合のみ付ける（例: `notion`, `store`, `tray`, `docs`）。
- `summary` は命令形で簡潔に（例: `add`, `fix`, `update`）。末尾に句点は付けない。
- 1 行は 72 文字程度までを目安にする。

## Type 一覧
- `feat`: 新機能追加
- `fix`: バグ修正
- `docs`: ドキュメントのみ
- `refactor`: 挙動を変えない内部改善
- `test`: テスト追加・修正
- `chore`: その他雑務（依存更新など）
- `build`: ビルド/依存/生成物の調整
- `ci`: CI 設定の変更
- `perf`: 性能改善
- `style`: フォーマットのみ（コード挙動変更なし）
- `revert`: 取り消し

## 本文とフッター（任意）
- 詳細説明が必要な場合は空行の後に本文を書く。
- 破壊的変更はフッターに `BREAKING CHANGE:` を付ける。

## 例
```
feat(tray): add refresh menu action
fix(notion): handle empty data source id
docs: add commit conventions
refactor(store): extract config path helper
```
