package app

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"nudge/internal/dto"
	"nudge/internal/notion"
)

// historyWeeks は連続達成週数を数えるときに遡る週の数（暦で 52 週前まで）。
const historyWeeks = 52

const daysPerWeek = 7

var goalPattern = regexp.MustCompile(`^週\s*([0-9]+)\s*回$`)

// fullWidthDigits は全角数字を半角に直す。
var fullWidthDigits = strings.NewReplacer(
	"０", "0", "１", "1", "２", "2", "３", "3", "４", "4",
	"５", "5", "６", "6", "７", "7", "８", "8", "９", "9",
)

// parseGoal は「週N回」の N を返す。1〜7 以外や読めない値は nil（詳細設計 1.3）。
func parseGoal(value string) *int {
	normalized := fullWidthDigits.Replace(strings.TrimSpace(value))
	m := goalPattern.FindStringSubmatch(normalized)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n < 1 || n > daysPerWeek {
		return nil
	}
	return &n
}

// weekStart は t が属する週（日曜始まり）の日曜 0:00 を、t のタイムゾーンで返す。
func weekStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d-int(t.Weekday()), 0, 0, 0, 0, t.Location())
}

// habitSummaryResult は週集計と、そこから作る今日の一覧・件数。
type habitSummaryResult struct {
	Summary   dto.HabitSummary
	Today     []dto.Task
	Remaining int
}

// buildHabitSummary は今週のページと過去のページから週集計を作る（詳細設計 4 章）。
// weekLoaded は今週分の取得が一度でも成功したか、history は過去分（未取得なら nil）。
func buildHabitSummary(week []notion.HabitPage, weekLoaded bool, history []notion.HabitPage, historyLoaded bool, now time.Time) habitSummaryResult {
	todayIndex := int(now.Weekday())
	result := habitSummaryResult{Summary: dto.HabitSummary{
		Enabled:        true,
		TodayIndex:     todayIndex,
		HistoryLoading: !historyLoaded,
		Rows:           []dto.HabitWeekRow{},
	}}
	if !weekLoaded {
		result.Summary.Reason = dto.HabitSummaryReasonNotLoaded
		return result
	}
	thisWeek := weekStart(now)
	current := latestPerHabit(week, now.Location(), thisWeek)
	if len(current) == 0 {
		result.Summary.Reason = dto.HabitSummaryReasonNoWeekPages
		return result
	}
	past := pastPagesByWeek(history, now.Location())

	for _, page := range current {
		row := buildWeekRow(page, todayIndex)
		if historyLoaded && row.Goal != nil {
			streak, capped := countStreak(past[page.Title], thisWeek, row)
			row.Streak = &streak
			row.StreakCapped = capped
		}
		result.Summary.Rows = append(result.Summary.Rows, row)
		if row.Days[todayIndex] {
			continue
		}
		result.Today = append(result.Today, dto.Task{ID: page.ID, Title: page.Title, URL: page.URL, Risk: row.Risk})
		if row.Goal == nil || row.CheckedCount < *row.Goal {
			result.Remaining++
		}
	}
	return result
}

// latestPerHabit は今週のページを習慣名でまとめ、作成日時が最も新しい 1 件を、新しい順に返す。
func latestPerHabit(pages []notion.HabitPage, loc *time.Location, thisWeek time.Time) []notion.HabitPage {
	latest := make(map[string]notion.HabitPage)
	for _, p := range pages {
		if p.Title == "" || p.CreatedTime.In(loc).Before(thisWeek) {
			continue
		}
		if cur, ok := latest[p.Title]; !ok || p.CreatedTime.After(cur.CreatedTime) {
			latest[p.Title] = p
		}
	}
	out := make([]notion.HabitPage, 0, len(latest))
	for _, p := range latest {
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].CreatedTime.Equal(out[j].CreatedTime) {
			return out[i].CreatedTime.After(out[j].CreatedTime)
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// pastPagesByWeek は過去のページを（習慣名 → 週の始まりの日付 → 最新の 1 件）にまとめる。
// 週は日付の文字列（YYYY-MM-DD）で見分ける（time.Time の内部表現の違いに左右されないため）。
func pastPagesByWeek(pages []notion.HabitPage, loc *time.Location) map[string]map[string]notion.HabitPage {
	out := make(map[string]map[string]notion.HabitPage)
	for _, p := range pages {
		if p.Title == "" {
			continue
		}
		ws := weekStart(p.CreatedTime.In(loc)).Format(dateLayout)
		byWeek, ok := out[p.Title]
		if !ok {
			byWeek = make(map[string]notion.HabitPage)
			out[p.Title] = byWeek
		}
		if cur, ok := byWeek[ws]; !ok || p.CreatedTime.After(cur.CreatedTime) {
			byWeek[ws] = p
		}
	}
	return out
}

func buildWeekRow(page notion.HabitPage, todayIndex int) dto.HabitWeekRow {
	row := dto.HabitWeekRow{
		Name:         page.Title,
		URL:          page.URL,
		Days:         page.Days[:],
		CheckedCount: countChecked(page.Days),
		Goal:         parseGoal(page.Goal),
	}
	row.Risk = habitRisk(row, todayIndex)
	return row
}

func countChecked(days [notion.WeekdayCount]bool) int {
	n := 0
	for _, d := range days {
		if d {
			n++
		}
	}
	return n
}

// habitRisk は今日やらないと今週の目標に届かなくなるかを判定する（詳細設計 4 章 4）。
func habitRisk(row dto.HabitWeekRow, todayIndex int) string {
	if row.Goal == nil || row.Days[todayIndex] {
		return ""
	}
	remainingCount := *row.Goal - row.CheckedCount
	remainingDays := daysPerWeek - todayIndex
	switch {
	case remainingCount <= 0:
		return ""
	case remainingCount == remainingDays:
		return dto.HabitRiskAtRisk
	case remainingCount > remainingDays:
		return dto.HabitRiskUnreachable
	default:
		return ""
	}
}

// countStreak は先週から 52 週前まで遡って達成週の連続を数え、今週が達成済みなら 1 を足す。
// ページが無い週とゴールが読めない週は飛ばす。52 週前にもページがあって一度も途切れなければ capped。
func countStreak(byWeek map[string]notion.HabitPage, thisWeek time.Time, current dto.HabitWeekRow) (int, bool) {
	streak := 0
	broken := false
	for k := 1; k <= historyWeeks; k++ {
		page, ok := byWeek[thisWeek.AddDate(0, 0, -daysPerWeek*k).Format(dateLayout)]
		if !ok {
			continue
		}
		goal := parseGoal(page.Goal)
		if goal == nil {
			continue
		}
		if countChecked(page.Days) < *goal {
			broken = true
			break
		}
		streak++
	}
	_, hasOldest := byWeek[thisWeek.AddDate(0, 0, -daysPerWeek*historyWeeks).Format(dateLayout)]
	capped := !broken && hasOldest
	if current.Goal != nil && current.CheckedCount >= *current.Goal {
		streak++
	}
	return streak, capped
}
