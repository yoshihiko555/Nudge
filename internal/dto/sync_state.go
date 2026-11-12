package dto

// 取得失敗の種類（詳細設計 1.4）。
const (
	ErrorKindAuth        = "auth"
	ErrorKindNotShared   = "not_shared"
	ErrorKindRateLimited = "rate_limited"
	ErrorKindServer      = "server"
	ErrorKindNetwork     = "network"
	ErrorKindConfig      = "config"
	ErrorKindUnknown     = "unknown"
)

// SyncState は DB ごとの最終取得の結果（API-006 の応答の要素）。
type SyncState struct {
	DatabaseKey   string `json:"database_key"`
	Name          string `json:"name"`
	OK            bool   `json:"ok"`
	LastAttemptAt string `json:"last_attempt_at"`
	LastSuccessAt string `json:"last_success_at"`
	ErrorKind     string `json:"error_kind"`
	// 過去分の習慣（連続達成週数用）の取得状態。対象外・未試行は HistoryOK が nil。
	HistoryOK         *bool  `json:"history_ok"`
	HistoryFetchedFor string `json:"history_fetched_for"`
	HistoryErrorKind  string `json:"history_error_kind"`
	// 直近の取得で件数の上限により打ち切ったか。
	Truncated bool `json:"truncated"`
}
