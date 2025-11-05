package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the platform specific configuration directory for resterm.
func Dir() string {
	if override := os.Getenv("RESTERM_CONFIG_DIR"); override != "" {
		return override
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ".resterm"
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "resterm")
	case "windows":
		return filepath.Join(home, "AppData", "Roaming", "resterm")
	default:
		return filepath.Join(home, ".config", "resterm")
	}
}

// HistoryPath returns the absolute path to the history file.
func HistoryPath() string {
	return filepath.Join(Dir(), "history.json")
}
