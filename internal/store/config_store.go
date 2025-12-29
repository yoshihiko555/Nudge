package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"nudge/internal/dto"
)

type ConfigStore interface {
	Load() (dto.Config, error)
	Save(cfg dto.Config) error
	Path() (string, error)
}

type FileConfigStore struct {
	AppName string
}

func NewFileConfigStore(appName string) *FileConfigStore {
	return &FileConfigStore{AppName: appName}
}

func (s *FileConfigStore) Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, s.AppName, "config.json"), nil
}

func (s *FileConfigStore) Load() (dto.Config, error) {
	cfg := dto.DefaultConfig()
	path, err := s.Path()
	if err != nil {
		return cfg, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

func (s *FileConfigStore) Save(cfg dto.Config) error {
	path, err := s.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
