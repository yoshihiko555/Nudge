package notion

import (
	"encoding/json"
	"fmt"
	"strings"
)

// APIError は Notion API が 2xx 以外を返したときのエラー。
// Error() の文言は v1 と同じ形にして、既存の表示を変えない。
type APIError struct {
	Status  int
	Code    string
	Message string
	Body    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("notion error: status=%d body=%s", e.Status, e.Body)
}

func newAPIError(status int, body []byte) *APIError {
	trimmed := strings.TrimSpace(string(body))
	e := &APIError{Status: status, Body: trimmed}
	var parsed struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		e.Code = parsed.Code
		e.Message = parsed.Message
	}
	return e
}
