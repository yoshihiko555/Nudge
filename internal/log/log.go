package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type Options struct {
	AppName string
	Level   slog.Level
}

func New(opts Options) (*slog.Logger, func() error, error) {
	if opts.AppName == "" {
		return nil, nil, fmt.Errorf("app name is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, nil, fmt.Errorf("user home dir: %w", err)
	}
	logDir := filepath.Join(home, "Library", "Logs", opts.AppName)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("mkdir log dir: %w", err)
	}
	path := filepath.Join(logDir, "app.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}
	cleanup := func() error {
		return f.Close()
	}
	handler := slog.NewTextHandler(io.MultiWriter(os.Stdout, f), &slog.HandlerOptions{Level: opts.Level})
	return slog.New(handler), cleanup, nil
}
