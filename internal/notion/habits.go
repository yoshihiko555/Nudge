package notion

import (
	"context"
	"strings"
	"time"

	"nudge/internal/dto"
)

// WeekdayCount は曜日チェック列の数（日〜土）。
const WeekdayCount = 7

// HabitPage は習慣 DB の 1 ページ（1 つの習慣の 1 週分）。
type HabitPage struct {
	ID          string
	URL         string
	Title       string
	CreatedTime time.Time
	// Days は日〜土のチェック状態。
	Days [WeekdayCount]bool
	// Goal はゴールのプロパティの値（Select の名前か rich_text の文字列）。読めなければ空。
	Goal string
}

// QueryHabitPages は作成日時が from 以上（before が空でなければ before 未満）の習慣のページを全件取る。
// weekdays は日〜土の 7 列のプロパティ名。
func (c *Client) QueryHabitPages(ctx context.Context, db dto.DatabaseConfig, notionVersion string, weekdays []string, from, before time.Time) ([]HabitPage, bool, error) {
	if err := db.ValidateForHabit(notionVersion); err != nil {
		return nil, false, err
	}
	body := map[string]any{
		"filter": buildCreatedTimeFilter(from, before),
		"sorts": []map[string]any{{
			"timestamp": "created_time",
			"direction": "descending",
		}},
	}
	pages, truncated, err := c.queryAll(ctx, db.DataSourceID, body, notionVersion)
	if err != nil {
		return nil, false, err
	}
	return mapHabitPages(pages, db, weekdays), truncated, nil
}

// buildCreatedTimeFilter は作成日時の範囲の条件を組む。日時はオフセット付きの RFC 3339 で渡す。
func buildCreatedTimeFilter(from, before time.Time) map[string]any {
	onOrAfter := map[string]any{
		"timestamp":    "created_time",
		"created_time": map[string]any{"on_or_after": from.Format(time.RFC3339)},
	}
	if before.IsZero() {
		return onOrAfter
	}
	return map[string]any{
		"and": []map[string]any{
			onOrAfter,
			{
				"timestamp":    "created_time",
				"created_time": map[string]any{"before": before.Format(time.RFC3339)},
			},
		},
	}
}

func mapHabitPages(pages []page, db dto.DatabaseConfig, weekdays []string) []HabitPage {
	out := make([]HabitPage, 0, len(pages))
	for _, p := range pages {
		created, err := time.Parse(time.RFC3339Nano, p.CreatedTime)
		if err != nil {
			continue
		}
		habit := HabitPage{
			ID:          p.ID,
			URL:         p.URL,
			Title:       strings.TrimSpace(extractTitle(p.Properties[db.TitlePropertyName])),
			CreatedTime: created,
		}
		for i, name := range weekdays {
			if i >= WeekdayCount {
				break
			}
			habit.Days[i] = extractCheckbox(p.Properties[name])
		}
		if db.GoalPropertyName != "" {
			habit.Goal = extractGoal(p.Properties[db.GoalPropertyName])
		}
		out = append(out, habit)
	}
	return out
}

// extractGoal はゴールの値を読む。Select ならその名前、rich_text ならつなげた文字列。
func extractGoal(prop propertyValue) string {
	switch prop.Type {
	case "select":
		return extractSelectName(prop)
	case "rich_text":
		return joinPlainText(prop.RichText)
	default:
		return ""
	}
}
