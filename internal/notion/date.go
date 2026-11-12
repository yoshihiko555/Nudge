package notion

import (
	"strings"
	"time"
)

const (
	dateOnlyLayout     = "2006-01-02"
	localDateTimeMilli = "2006-01-02T15:04:05.000"
	localDateTime      = "2006-01-02T15:04:05"
)

// dueDateOf は Date プロパティの値から、期日として使うローカル日付（YYYY-MM-DD）を返す。
// 期間なら終了日を使う。読めなければ空を返す（詳細設計 1.2）。
func dueDateOf(d *dateValue, loc *time.Location) string {
	if d == nil {
		return ""
	}
	value := d.Start
	if d.End != nil && strings.TrimSpace(*d.End) != "" {
		value = *d.End
	}
	tz := ""
	if d.TimeZone != nil {
		tz = *d.TimeZone
	}
	date, ok := parseNotionDate(value, tz, loc)
	if !ok {
		return ""
	}
	return date
}

// parseNotionDate は Notion の日付文字列を loc の日付（YYYY-MM-DD）に直す。
// - 日付だけの値はそのまま使う
// - オフセット付きの日時は loc に直してから日付を取る
// - オフセット無しの日時は timeZone（無ければ loc）の時刻として読む
func parseNotionDate(value, timeZone string, loc *time.Location) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if loc == nil {
		loc = time.Local
	}
	// 日付だけの値は、time_zone があってもタイムゾーンの変換をしない（その日付をそのまま期日とする）。
	if len(value) == len(dateOnlyLayout) {
		if _, err := time.Parse(dateOnlyLayout, value); err != nil {
			return "", false
		}
		return value, true
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.In(loc).Format(dateOnlyLayout), true
	}
	src := loc
	if timeZone != "" {
		if zone, err := time.LoadLocation(timeZone); err == nil {
			src = zone
		}
	}
	for _, layout := range []string{localDateTimeMilli, localDateTime} {
		if t, err := time.ParseInLocation(layout, value, src); err == nil {
			return t.In(loc).Format(dateOnlyLayout), true
		}
	}
	return "", false
}
