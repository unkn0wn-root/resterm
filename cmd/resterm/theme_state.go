package main

import (
	"fmt"
	"path/filepath"

	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/theme"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

type themeState struct {
	settings config.Settings
	handle   config.SettingsHandle
	catalog  theme.Catalog
	def      theme.Definition
	active   string
}

func loadThemeState() (themeState, error) {
	st := themeState{
		settings: config.Settings{},
		handle: config.SettingsHandle{
			Path:   filepath.Join(config.Dir(), "settings.toml"),
			Format: config.SettingsFormatTOML,
		},
		catalog: theme.Catalog{},
		def:     theme.DefaultDefinition(),
		active:  "default",
	}

	var errs []error

	cfg, h, cfgErr := config.LoadSettings()
	if cfgErr != nil {
		errs = append(errs, fmt.Errorf("settings load error: %w", cfgErr))
	} else {
		st.settings = cfg
		st.handle = h
	}

	cat, catErr := theme.LoadCatalog([]string{config.ThemeDir()})
	if catErr != nil {
		errs = append(errs, fmt.Errorf("theme load error: %w", catErr))
	}
	st.catalog = cat

	key := str.LowerTrim(st.settings.DefaultTheme)
	if key == "" {
		key = "default"
	}

	if def, ok := st.catalog.Get(key); ok {
		st.def = def
		st.active = def.Key
		st.settings.DefaultTheme = def.Key
		return st, diag.Join(diag.ClassConfig, "load theme state", errs...)
	}

	if st.settings.DefaultTheme != "" {
		errs = append(
			errs,
			fmt.Errorf("theme %q not found; using built-in default", st.settings.DefaultTheme),
		)
	}

	if def, ok := st.catalog.Get("default"); ok {
		st.def = def
		st.active = def.Key
	} else {
		st.def = theme.DefaultDefinition()
		st.active = st.def.Key
	}
	st.settings.DefaultTheme = ""
	return st, diag.Join(diag.ClassConfig, "load theme state", errs...)
}
