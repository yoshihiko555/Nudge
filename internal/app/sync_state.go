package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

// syncRecord は DB ごとの最終取得の結果（ポーリング・強制取得の分）。
type syncRecord struct {
	attempted   bool
	ok          bool
	lastAttempt time.Time
	lastSuccess time.Time
	errorKind   string
	truncated   bool
}

func (r syncRecord) toDTO(db dto.DatabaseConfig) dto.SyncState {
	state := dto.SyncState{
		DatabaseKey: db.Key,
		Name:        db.Name,
		// まだ一度も取得していない DB は、失敗としては扱わない。
		OK:        r.ok || !r.attempted,
		ErrorKind: r.errorKind,
		Truncated: r.truncated,
	}
	if !r.lastAttempt.IsZero() {
		state.LastAttemptAt = r.lastAttempt.Format(time.RFC3339)
	}
	if !r.lastSuccess.IsZero() {
		state.LastSuccessAt = r.lastSuccess.Format(time.RFC3339)
	}
	return state
}

// classifyError は取得の失敗を種類に分ける（詳細設計 1.4）。
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, dto.ErrInvalidConfig) {
		return dto.ErrorKindConfig
	}
	var apiErr *notion.APIError
	if errors.As(err, &apiErr) {
		return classifyStatus(apiErr.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return dto.ErrorKindNetwork
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return dto.ErrorKindNetwork
	}
	return dto.ErrorKindUnknown
}

func classifyStatus(status int) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return dto.ErrorKindAuth
	case status == http.StatusNotFound:
		return dto.ErrorKindNotShared
	case status == http.StatusTooManyRequests:
		return dto.ErrorKindRateLimited
	case status >= http.StatusInternalServerError:
		return dto.ErrorKindServer
	default:
		return dto.ErrorKindUnknown
	}
}
