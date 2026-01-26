# ADR-0011: Homebrew Cask での配布と DMG 運用

## 決定
- macOS 配布は Homebrew Cask を採用し、配布物は GitHub Releases の DMG とする
- Cask は専用 Tap リポジトリで管理し、トークンは `nudge` とする
- リリースタグは `v{version}`、DMG 名は `Nudge-{version}.dmg` とする
- DMG 生成と SHA256 算出は `task dmg` / `task dmg:sha256` で行う

## 理由
- dotfiles で `brew install nudge` を前提に管理したい
- macOS の GUI アプリ配布は Cask が標準で、運用が単純
- GitHub Releases は安定した配布 URL を提供できる
- SHA256 により配布物の改ざん検知ができる

## 代替案
- DMG を配布して手動インストールのみとする
- Formula でバイナリ配布する（GUI アプリの運用に不向き）
- App Store 配布に切り替える（審査や運用負荷が増える）

## 影響
- リリースごとに DMG 生成・Releases 反映・Cask の SHA256 更新が必要
- Tap リポジトリの保守が追加される
- 署名/Notarization 未対応のため Gatekeeper 警告の可能性がある
