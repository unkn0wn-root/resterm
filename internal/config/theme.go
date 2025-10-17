package config

import (
	"os"
	"path/filepath"
)

const themeDirName = "themes"

func ThemeDir() string {
	if override := os.Getenv("RESTERM_THEMES_DIR"); override != "" {
		return override
	}
	return filepath.Join(Dir(), themeDirName)
}
