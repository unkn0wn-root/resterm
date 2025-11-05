package config

import (
	"os"
	"path/filepath"
)

const themeDirName = "themes"

// ThemeDir returns the folder where user themes are stored.
func ThemeDir() string {
	if override := os.Getenv("RESTERM_THEMES_DIR"); override != "" {
		return override
	}
	return filepath.Join(Dir(), themeDirName)
}
