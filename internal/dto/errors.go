package dto

import "errors"

// ErrInvalidConfig は設定の不備（必要なプロパティ名や Data Source ID が空など）を表す。
// 取得失敗の種類を分けるときに、Notion API の失敗と区別するために使う。
var ErrInvalidConfig = errors.New("invalid config")

// configError は既存のエラー文言を変えずに ErrInvalidConfig として判定できるようにする。
type configError struct {
	msg string
}

func (e *configError) Error() string { return e.msg }

func (e *configError) Is(target error) bool { return target == ErrInvalidConfig }

func invalidConfig(msg string) error {
	return &configError{msg: msg}
}
