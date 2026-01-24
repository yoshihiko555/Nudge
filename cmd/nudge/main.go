package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	coreapp "nudge/internal/app"
	"nudge/internal/dto"
	"nudge/internal/notion"
	"nudge/internal/store"
)

// UI の埋め込み資産（index.html/js/css）をバイナリに同梱する
//
//go:embed assets/*
var assets embed.FS

const (
	// メニューバー（SystemTray）用のフォールバックアイコン（PNG を base64 化）
	// tray_icon_path が未設定・読み込み失敗時に使う
	trayIconBase64 = "iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAAAXNSR0IArs4c6QAAAHhlWElmTU0AKgAAAAgABAEaAAUAAAABAAAAPgEbAAUAAAABAAAARgEoAAMAAAABAAIAAIdpAAQAAAABAAAATgAAAAAAAABIAAAAAQAAAEgAAAABAAOgAQADAAAAAQABAACgAgAEAAAAAQAAACCgAwAEAAAAAQAAACAAAAAAnwlWxAAAAAlwSFlzAAALEwAACxMBAJqcGAAABDJJREFUWAnFV10oZWsYXtv/X9JgojONGadjOqcjbpQLuXRx4shMcaFRhPx1dC5QJEQJSeFCEunkYm4oTXLhghElZUZycVzoKCRi/J2z/e73PO871teasda2ttrNW+9e3/e8f8/6/ta3Ne07i8NdfSLyhd2tj5t4urORw+FwWfndS46i4XB+C/0N+gPUB/oYUQQQvA+dho6AzGfLZCj+K/Qj1FuyhsSJpgRgeAr921uVDXm5xlOdhHF4/wSYoBu8+OQaf+j5hQAYBQF4rYNWz+3tbW1zc9PK7An+BjUDVQA6z6FnUFO5vb2lhoYGCg4OJl9fXyosLKSrqytTX5vgv/B7YSTwI4D/rIJnZmZ4RVNzczP19/fztqLy8nIrdzs4s//FNgEUdfn7+9Px8bEkHxoaEkJtbW12ipn5eEZgbW2NAgMDqaSkRCVramoSEiMjIwrzoPEwgevraxobG6OdnR3JOzg4KAVbW1ul73K5qKioSNbE9PS0B7XF9RK/P7udgtPTU4qNjaXU1FQ6OTmRqMbGRiExOjoqfafTSRkZGRQeHk4rKyuCGX92d3eJ1UQeJsBB8/PzFBQURLm5uXRzc0P6WwcEBNDCwoLkPTg4oMTERIqPj6f9/X3BDg8PKT8/X2J56jie/QxijwAHDA8Py1tPTU1JPL91UlISpaenE29PlvX1deJF2tHRIf2CggIp3tnZST09PRQWFkZ5eXnyAuJAZJ8AF4yOjqaampq7WKLu7m4KCQkhflNd0tLSKDMzk87Pz9nmamlp0U1MwsWjtre3p2OKgPEoVmvi2waiND8/PwVjOqSPQ0lhGA0NRTQfHx8NuOPi4kLZ0HYwzmoqKPAT1KnTMz77+vpkCmZnZwXmsyAhIYGysrKU29LSEiE5DQwMCFZVVSX92tpaqq+vl+kpLi5W/mioERBCAEwJTE5OyqlXWloq88fHb05ODoWGhtLq6qok3Nraori4OEpOTibeOSw8DdXV1TJ1UVFRVFlZqWzi8IXAKzUaAO8R4GQ897zNeB2w8JshiCYmJqTPo5GSkkIxMTG0sbEhmPHn7Ozs28K6mUfAPQFe4fwNODo6kiBe4VycvwUsPBrZ2dnygVpcXBTMg5+HCRiTLS8vy4lXV1en4IqKCiE0Pj6uMA8anhHo7e2VbaTPcXt7uxTnBfpI8YzA3NycFCwrK5NVzVNhHI1HkLhH4BmSfFnCJtn4CO7q6iJe0Xzu86Gkn4Im7nYgvpC85F0g13J0/NFehiYxaCVY9RofOJGRkVYudvENOCbjYuOU4w2Na5D4C6BbAhEREXYLPOT3jot/5QQCodAPUG/LJxR48lVxvQNDDPS9FxnweS5zr9c0+2vGX4zfoXxN5zs8X595F1iJ7BALI9tuoP9AJ6HjGPpLPJXcI6Asdw0wtviEKU935DQUdGtXWb5X43+lFriav9FvawAAAABJRU5ErkJggg=="
)

type rpcRequest struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

type rpcResponse struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

type getTasksPayload struct {
	DatabaseKey string `json:"database_key"`
}

type updateStatusPayload struct {
	DatabaseKey string `json:"database_key"`
	TaskID      string `json:"task_id"`
	Action      string `json:"action"` // done | paused | resume
}

type getHabitsPayload struct {
	DatabaseKey string `json:"database_key"`
}

type updateHabitPayload struct {
	DatabaseKey string `json:"database_key"`
	TaskID      string `json:"task_id"`
	Checked     bool   `json:"checked"`
}

type resolvePayload struct {
	DatabaseID string `json:"database_id"`
}

type openURLPayload struct {
	URL string `json:"url"`
}

type createBrainPagePayload struct {
	Body string `json:"body"`
}

func main() {
	// 永続化ストアと Notion クライアントの組み立て
	cfgStore := store.NewFileConfigStore(coreapp.AppName)
	tokenStore := store.NewKeychainTokenStore(coreapp.KeychainService, coreapp.KeychainAccount)
	notionClient := notion.NewClient(tokenStore)
	core := coreapp.NewApp(cfgStore, tokenStore, notionClient)
	_, _ = core.LoadConfig()

	var app *application.App
	var settingsWindow *application.WebviewWindow
	var brainWindow *application.WebviewWindow
	app = application.New(application.Options{
		Name:        coreapp.AppName,
		Description: "Notion tasks in menu bar",
		Assets: application.AssetOptions{
			// 埋め込み assets をローカルサーバとして提供
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Dock に表示しない（メニューバー専用アプリ）
			ActivationPolicy: application.ActivationPolicyAccessory,
			// メニューバー常駐のため、最後のウィンドウが閉じても終了しない
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// JS 側からの RPC を直接ハンドリング
		RawMessageHandler: func(window application.Window, message string, origin *application.OriginInfo) {
			handleRawMessage(core, app, settingsWindow, brainWindow, window, message, origin)
		},
	})

	popover := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          "popover",
		Title:         coreapp.AppName,
		Width:         380,
		Height:        540,
		Hidden:        true,
		Frameless:     true,
		AlwaysOnTop:   true,
		DisableResize: true,
		URL:           "/index.html",
	})

	settingsWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "settings",
		Title:     coreapp.AppName + " 設定",
		Width:     640,
		Height:    720,
		MinWidth:  520,
		MinHeight: 600,
		Hidden:    true,
		URL:       "/index.html?mode=settings",
	})
	settingsWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		settingsWindow.Hide()
	})

	brainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "brain",
		Title:     coreapp.AppName + " Brain",
		Width:     520,
		Height:    640,
		MinWidth:  480,
		MinHeight: 560,
		Hidden:    true,
		URL:       "/index.html?mode=brain",
	})
	brainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		brainWindow.Hide()
	})

	// メニューバー（SystemTray）の初期化
	setupTray(app, popover, settingsWindow, core.GetConfig())

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupTray(app *application.App, window *application.WebviewWindow, settingsWindow *application.WebviewWindow, cfg dto.Config) {
	// パス指定があれば PNG を直接読み込む（base64 変換不要）
	icon := loadTrayIcon(cfg)
	// メニューバー（SystemTray）を構成
	systray := app.SystemTray.New()
	if icon != nil {
		systray.SetIcon(icon)
		systray.SetDarkModeIcon(icon)
	}
	// ラベルを空にしてアイコンのみ表示
	systray.SetLabel("")

	menu := app.NewMenu()
	menu.Add("タスク").OnClick(func(ctx *application.Context) {
		window.EmitEvent("view-change", "tasks")
		window.Show()
	})
	menu.Add("習慣").OnClick(func(ctx *application.Context) {
		window.EmitEvent("view-change", "habits")
		window.Show()
	})
	menu.Add("設定").OnClick(func(ctx *application.Context) {
		showSettingsWindow(settingsWindow)
	})
	menu.AddSeparator()
	menu.Add("更新").OnClick(func(ctx *application.Context) {
		window.EmitEvent("refresh")
	})
	menu.AddSeparator()
	menu.Add("終了").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	// メニュー適用とウィンドウの紐付け
	systray.SetMenu(menu)
	systray.AttachWindow(window)
}

func loadTrayIcon(cfg dto.Config) []byte {
	if strings.TrimSpace(cfg.TrayIconPath) != "" {
		path := resolveTrayIconPath(cfg.TrayIconPath)
		b, err := os.ReadFile(path)
		if err != nil {
			log.Printf("tray icon: read failed: path=%s err=%v", path, err)
		} else if len(b) > 0 {
			return b
		}
	}
	// フォールバック（埋め込み base64）
	icon, err := base64.StdEncoding.DecodeString(trayIconBase64)
	if err != nil {
		log.Printf("tray icon: base64 decode failed: %v", err)
		return nil
	}
	return icon
}

func resolveTrayIconPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return path
	}
	return filepath.Join(base, coreapp.AppName, path)
}

func handleRawMessage(core *coreapp.App, app *application.App, settingsWindow *application.WebviewWindow, brainWindow *application.WebviewWindow, window application.Window, message string, origin *application.OriginInfo) {
	// 外部起点のメッセージは拒否
	if !isTrustedOrigin(origin) {
		return
	}
	var req rpcRequest
	if err := json.Unmarshal([]byte(message), &req); err != nil {
		return
	}
	if req.ID == "" || req.Action == "" {
		return
	}

	ctx := context.Background()
	respond := func(resp rpcResponse) {
		// 返信はイベント経由でフロントエンドに返す
		window.EmitEvent("rpc:response", resp)
	}

	// action に応じてアプリ側ユースケースを呼び分け
	switch req.Action {
	case "getConfig":
		cfg, err := core.LoadConfig()
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: cfg})
	case "saveConfig":
		var cfg dto.Config
		if err := json.Unmarshal(req.Payload, &cfg); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		if err := core.SaveConfig(cfg); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true})
	case "getTokenStatus":
		token, err := core.GetToken()
		if err != nil {
			if errors.Is(err, store.ErrTokenNotFound) {
				respond(rpcResponse{ID: req.ID, OK: true, Data: false})
				return
			}
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: token != ""})
	case "setToken":
		var payload struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		if err := core.SetToken(payload.Token); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true})
	case "clearToken":
		if err := core.ClearToken(); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true})
	case "resolveDataSourceID":
		var payload resolvePayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		id, err := core.ResolveDataSourceID(ctx, payload.DatabaseID)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: id})
	case "resolveTitlePropertyName":
		var payload resolvePayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		name, err := core.ResolveTitlePropertyName(ctx, payload.DatabaseID)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: name})
	case "getBrainTemplate":
		tpl, err := core.GetBrainTemplate(ctx)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: tpl})
	case "createBrainPage":
		var payload createBrainPagePayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		page, err := core.CreateBrainPage(ctx, payload.Body)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: page})
	case "getTasks":
		var payload getTasksPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		tasks, err := core.QueryTasks(ctx, payload.DatabaseKey)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: tasks})
	case "getHabits":
		var payload getHabitsPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		habits, err := core.QueryHabits(ctx, payload.DatabaseKey)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: habits})
	case "updateStatus":
		var payload updateStatusPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		if err := core.UpdateTaskStatus(ctx, payload.DatabaseKey, payload.TaskID, payload.Action); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true})
	case "updateHabitCheck":
		var payload updateHabitPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		if err := core.UpdateHabitCheck(ctx, payload.DatabaseKey, payload.TaskID, payload.Checked); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true})
	case "openURL":
		var payload openURLPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		if payload.URL == "" {
			respond(rpcResponse{ID: req.ID, OK: false, Error: "url is empty"})
			return
		}
		if err := app.Browser.OpenURL(payload.URL); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true})
	case "openSettingsWindow":
		if settingsWindow == nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: "settings window unavailable"})
			return
		}
		showSettingsWindow(settingsWindow)
		respond(rpcResponse{ID: req.ID, OK: true})
	case "openBrainWindow":
		if brainWindow == nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: "brain window unavailable"})
			return
		}
		showBrainWindow(brainWindow)
		respond(rpcResponse{ID: req.ID, OK: true})
	default:
		respond(rpcResponse{ID: req.ID, OK: false, Error: "unknown action"})
	}
}

func showSettingsWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.Show()
	window.Focus()
}

func showBrainWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.Show()
	window.Focus()
}

func isTrustedOrigin(origin *application.OriginInfo) bool {
	if origin == nil {
		return true
	}
	if !origin.IsMainFrame {
		return false
	}
	if origin.Origin == "" {
		return true
	}
	// 埋め込み webview 由来のみ許可
	return strings.HasPrefix(origin.Origin, "wails://") || strings.HasPrefix(origin.Origin, "http://wails.localhost")
}
