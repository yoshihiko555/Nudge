//go:build darwin

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func setLaunchAtLogin(enabled bool) error {
	appPath, err := appBundlePath()
	if err != nil {
		return err
	}
	script := buildLoginItemScript(AppName, appPath, enabled)
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("login item: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func appBundlePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("executable path: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	needle := ".app" + string(filepath.Separator)
	idx := strings.Index(exe, needle)
	if idx == -1 {
		return "", fmt.Errorf("app bundle not found: %s", exe)
	}
	return exe[:idx+len(".app")], nil
}

func buildLoginItemScript(appName string, appPath string, enabled bool) string {
	name := appleScriptString(appName)
	path := appleScriptString(appPath)
	if enabled {
		return fmt.Sprintf(`tell application "System Events"
  set loginItemName to "%s"
  set loginItemPath to "%s"
  if (exists login item loginItemName) then
    set path of login item loginItemName to loginItemPath
    set hidden of login item loginItemName to false
  else
    make login item at end with properties {name:loginItemName, path:loginItemPath, hidden:false}
  end if
end tell`, name, path)
	}
	return fmt.Sprintf(`tell application "System Events"
  set loginItemName to "%s"
  set loginItemPath to "%s"
  if (exists login item loginItemName) then
    delete login item loginItemName
  end if
  repeat with li in (every login item whose path is loginItemPath)
    delete li
  end repeat
end tell`, name, path)
}

func appleScriptString(value string) string {
	return strings.ReplaceAll(value, "\"", "\\\"")
}
