package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	coreapp "nudge/internal/app"
	"nudge/internal/dto"
	"nudge/internal/notion"
	"nudge/internal/store"
)

//go:embed assets/*
var assets embed.FS

const (
	trayIconBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO5WlHkAAAAASUVORK5CYII="
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
	Status string `json:"status"`
}

type updateStatusPayload struct {
	TaskID string `json:"task_id"`
	Action string `json:"action"` // done | paused | resume
}

type resolvePayload struct {
	DatabaseID string `json:"database_id"`
}

type openURLPayload struct {
	URL string `json:"url"`
}

func main() {
	cfgStore := store.NewFileConfigStore(coreapp.AppName)
	tokenStore := store.NewKeychainTokenStore(coreapp.KeychainService, coreapp.KeychainAccount)
	notionClient := notion.NewClient(tokenStore)
	core := coreapp.NewApp(cfgStore, tokenStore, notionClient)
	_, _ = core.LoadConfig()

	var app *application.App
	app = application.New(application.Options{
		Name:        coreapp.AppName,
		Description: "Notion tasks in menu bar",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		RawMessageHandler: func(window application.Window, message string, origin *application.OriginInfo) {
			handleRawMessage(core, app, window, message, origin)
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

	setupTray(app, popover)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func setupTray(app *application.App, window *application.WebviewWindow) {
	icon, err := base64.StdEncoding.DecodeString(trayIconBase64)
	if err != nil {
		icon = nil
	}
	systray := app.SystemTray.New()
	if icon != nil {
		systray.SetIcon(icon)
		systray.SetDarkModeIcon(icon)
	}
	systray.SetLabel(coreapp.AppName)

	menu := app.NewMenu()
	menu.Add("進行中").OnClick(func(ctx *application.Context) {
		window.EmitEvent("view-change", "inprogress")
		window.Show()
	})
	menu.Add("中断").OnClick(func(ctx *application.Context) {
		window.EmitEvent("view-change", "paused")
		window.Show()
	})
	menu.Add("設定").OnClick(func(ctx *application.Context) {
		window.EmitEvent("view-change", "settings")
		window.Show()
	})
	menu.AddSeparator()
	menu.Add("更新").OnClick(func(ctx *application.Context) {
		window.EmitEvent("refresh")
	})
	menu.AddSeparator()
	menu.Add("終了").OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	systray.SetMenu(menu)
	systray.AttachWindow(window)
}

func handleRawMessage(core *coreapp.App, app *application.App, window application.Window, message string, origin *application.OriginInfo) {
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
		window.EmitEvent("rpc:response", resp)
	}

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
		statusValue := mapStatus(core.GetConfig(), payload.Status)
		if statusValue == "" {
			respond(rpcResponse{ID: req.ID, OK: false, Error: "status is not configured"})
			return
		}
		tasks, err := core.QueryByStatus(ctx, statusValue)
		if err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		respond(rpcResponse{ID: req.ID, OK: true, Data: tasks})
	case "updateStatus":
		var payload updateStatusPayload
		if err := json.Unmarshal(req.Payload, &payload); err != nil {
			respond(rpcResponse{ID: req.ID, OK: false, Error: err.Error()})
			return
		}
		cfg := core.GetConfig()
		statusValue := mapActionToStatus(cfg, payload.Action)
		if statusValue == "" {
			respond(rpcResponse{ID: req.ID, OK: false, Error: "status is not configured"})
			return
		}
		if err := core.UpdateTaskStatus(ctx, payload.TaskID, statusValue); err != nil {
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
	default:
		respond(rpcResponse{ID: req.ID, OK: false, Error: "unknown action"})
	}
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
	return strings.HasPrefix(origin.Origin, "wails://") || strings.HasPrefix(origin.Origin, "http://wails.localhost")
}

func mapStatus(cfg dto.Config, status string) string {
	switch status {
	case "inprogress":
		return cfg.StatusInProgress
	case "paused":
		return cfg.StatusPaused
	default:
		return ""
	}
}

func mapActionToStatus(cfg dto.Config, action string) string {
	switch action {
	case "done":
		return cfg.StatusDone
	case "paused":
		return cfg.StatusPaused
	case "resume":
		return cfg.StatusInProgress
	default:
		return ""
	}
}
