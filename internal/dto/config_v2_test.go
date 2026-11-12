package dto

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalize_V2FieldsTrimAndDedupe(t *testing.T) {
	t.Parallel()

	cfg := Config{Databases: []DatabaseConfig{{
		Key:                  "tasks",
		DueDatePropertyName:  " 期限日 ",
		PriorityPropertyName: " 優先度 ",
		PriorityOrder:        []string{" 高 ", "", "中", "高", "低 "},
		GoalPropertyName:     " ゴール ",
	}}}
	got := cfg.Normalize().Databases[0]

	if got.DueDatePropertyName != "期限日" || got.PriorityPropertyName != "優先度" || got.GoalPropertyName != "ゴール" {
		t.Fatalf("property names not trimmed: %+v", got)
	}
	if want := []string{"高", "中", "低"}; !reflect.DeepEqual(got.PriorityOrder, want) {
		t.Fatalf("PriorityOrder = %v, want %v", got.PriorityOrder, want)
	}
}

func TestNormalize_V2EmptyPriorityPropertyClearsOrder(t *testing.T) {
	t.Parallel()

	cfg := Config{Databases: []DatabaseConfig{{
		Key:           "tasks",
		PriorityOrder: []string{"高", "中"},
	}}}
	if got := cfg.Normalize().Databases[0].PriorityOrder; got != nil {
		t.Fatalf("PriorityOrder = %v, want nil when priority property is empty", got)
	}
}

func TestNormalize_V2KeepsForeignKindKeys(t *testing.T) {
	t.Parallel()

	cfg := Config{Databases: []DatabaseConfig{{
		Key:                 "habits",
		Kind:                DatabaseKindHabit,
		DueDatePropertyName: "期限日",
	}}}
	if got := cfg.Normalize().Databases[0].DueDatePropertyName; got != "期限日" {
		t.Fatalf("task key on habit DB must be kept as is, got %q", got)
	}
}

func TestUsesDueDateQuery(t *testing.T) {
	t.Parallel()

	full := DatabaseConfig{DueDatePropertyName: "期限日", StatusInProgress: "進行中", StatusDone: "完了", StatusPaused: "中断"}
	if !full.UsesDueDateQuery() {
		t.Fatalf("full config must use due date query")
	}
	for name, mutate := range map[string]func(*DatabaseConfig){
		"no due":         func(d *DatabaseConfig) { d.DueDatePropertyName = "" },
		"no in progress": func(d *DatabaseConfig) { d.StatusInProgress = "" },
		"no done":        func(d *DatabaseConfig) { d.StatusDone = "" },
		"no paused":      func(d *DatabaseConfig) { d.StatusPaused = "" },
	} {
		d := full
		mutate(&d)
		if d.UsesDueDateQuery() {
			t.Fatalf("%s: must fall back to v1 query", name)
		}
	}
}

func TestValidateErrorsAreInvalidConfig(t *testing.T) {
	t.Parallel()

	err := DatabaseConfig{}.ValidateForTaskQuery("", "進行中")
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
	if err.Error() != "data_source_id is required" {
		t.Fatalf("message changed: %q", err.Error())
	}
}
