package store

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type TokenStore interface {
	GetToken() (string, error)
	SetToken(token string) error
	ClearToken() error
}

// KeychainTokenStore は macOS Keychain を使用する。
type KeychainTokenStore struct {
	Service string
	Account string
}

func NewKeychainTokenStore(service, account string) *KeychainTokenStore {
	return &KeychainTokenStore{Service: service, Account: account}
}

func (s *KeychainTokenStore) GetToken() (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", s.Service, "-a", s.Account, "-w")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "could not be found") {
			return "", ErrTokenNotFound
		}
		return "", fmt.Errorf("keychain get: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s *KeychainTokenStore) SetToken(token string) error {
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", s.Service, "-a", s.Account, "-w", token)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("keychain set: %w", err)
	}
	return nil
}

func (s *KeychainTokenStore) ClearToken() error {
	cmd := exec.Command("security", "delete-generic-password", "-s", s.Service, "-a", s.Account)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "could not be found") {
			return nil
		}
		return fmt.Errorf("keychain delete: %w", err)
	}
	return nil
}

var ErrTokenNotFound = errors.New("token not found")
