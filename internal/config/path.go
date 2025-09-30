package config

import (
	"os"
	"path/filepath"
	"runtime"
)

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

func HistoryPath() string {
	return filepath.Join(Dir(), "history.json")
}
