package main

import (
	"testing"

	"nudge/internal/dto"
)

func TestTrayDatabaseLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		db   dto.DatabaseConfig
		want string
	}{
		{
			name: "Nameが設定されている場合はそのまま返す",
			db: dto.DatabaseConfig{
				Name: "開発タスク",
				Kind: dto.DatabaseKindTask,
			},
			want: "開発タスク",
		},
		{
			name: "Nameが空でKindがtaskならタスクを返す",
			db: dto.DatabaseConfig{
				Name: "",
				Kind: dto.DatabaseKindTask,
			},
			want: defaultLabelTask,
		},
		{
			name: "Nameが空でKindがhabitなら習慣を返す",
			db: dto.DatabaseConfig{
				Name: "",
				Kind: dto.DatabaseKindHabit,
			},
			want: defaultLabelHabit,
		},
		{
			name: "Nameが空白のみでKindがtaskならタスクを返す",
			db: dto.DatabaseConfig{
				Name: "   ",
				Kind: dto.DatabaseKindTask,
			},
			want: defaultLabelTask,
		},
		{
			name: "Nameが空白のみでKindがhabitなら習慣を返す",
			db: dto.DatabaseConfig{
				Name: "\t \n",
				Kind: dto.DatabaseKindHabit,
			},
			want: defaultLabelHabit,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange
			db := tc.db

			// Act
			got := trayDatabaseLabel(db)

			// Assert
			if got != tc.want {
				t.Fatalf("trayDatabaseLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMenuLabelConstants(t *testing.T) {
	t.Parallel()

	if menuLabelSettings != "設定" {
		t.Fatalf("menuLabelSettings = %q, want %q", menuLabelSettings, "設定")
	}
	if menuLabelRefresh != "更新" {
		t.Fatalf("menuLabelRefresh = %q, want %q", menuLabelRefresh, "更新")
	}
	if menuLabelQuit != "終了" {
		t.Fatalf("menuLabelQuit = %q, want %q", menuLabelQuit, "終了")
	}
}
