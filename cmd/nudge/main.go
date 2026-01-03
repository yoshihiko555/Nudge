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
	trayIconBase64 = "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"
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

func main() {
	// 永続化ストアと Notion クライアントの組み立て
	cfgStore := store.NewFileConfigStore(coreapp.AppName)
	tokenStore := store.NewKeychainTokenStore(coreapp.KeychainService, coreapp.KeychainAccount)
	notionClient := notion.NewClient(tokenStore)
	core := coreapp.NewApp(cfgStore, tokenStore, notionClient)
	_, _ = core.LoadConfig()

	var app *application.App
	var settingsWindow *application.WebviewWindow
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
			handleRawMessage(core, app, settingsWindow, window, message, origin)
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

func handleRawMessage(core *coreapp.App, app *application.App, settingsWindow *application.WebviewWindow, window application.Window, message string, origin *application.OriginInfo) {
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
