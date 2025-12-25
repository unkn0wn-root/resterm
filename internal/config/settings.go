package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	SettingsFormatTOML SettingsFormat = "toml"
	SettingsFormatJSON SettingsFormat = "json"
)

type Settings struct {
	DefaultTheme string         `json:"default_theme" toml:"default_theme"`
	Layout       LayoutSettings `json:"layout"        toml:"layout"`
}

type SettingsFormat string
type SettingsHandle struct {
	Path   string
	Format SettingsFormat
}

// tries loading TOML first, then JSON, then returns empty settings if neither exists.
// parse errors fail immediately but missing files just skip to the next format.
func LoadSettings() (Settings, SettingsHandle, error) {
	dir := Dir()
	candidates := []SettingsHandle{
		{Path: filepath.Join(dir, "settings.toml"), Format: SettingsFormatTOML},
		{Path: filepath.Join(dir, "settings.json"), Format: SettingsFormatJSON},
	}

	var accumulated error
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			accumulated = errors.Join(
				accumulated,
				fmt.Errorf("read settings %q: %w", candidate.Path, err),
			)
			continue
		}

		settings, err := decodeSettings(data, candidate.Format)
		if err != nil {
			return Settings{}, SettingsHandle{}, fmt.Errorf(
				"parse settings %q: %w",
				candidate.Path,
				err,
			)
		}
		settings.Layout = NormaliseLayoutSettings(settings.Layout)
		return settings, candidate, nil
	}

	if accumulated != nil {
		return Settings{}, SettingsHandle{}, accumulated
	}

	return Settings{
			Layout: DefaultLayoutSettings(),
		}, SettingsHandle{
			Path:   candidates[0].Path,
			Format: SettingsFormatTOML,
		}, nil
}

func decodeSettings(data []byte, format SettingsFormat) (Settings, error) {
	var settings Settings
	switch format {
	case SettingsFormatTOML:
		if err := toml.Unmarshal(data, &settings); err != nil {
			return Settings{}, err
		}
	case SettingsFormatJSON:
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&settings); err != nil {
			return Settings{}, err
		}
	default:
		return Settings{}, fmt.Errorf("unsupported settings format %q", format)
	}
	return settings, nil
}

func SaveSettings(settings Settings, handle SettingsHandle) error {
	settings.Layout = NormaliseLayoutSettings(settings.Layout)
	path := handle.Path
	format := handle.Format
	if path == "" {
		path = filepath.Join(Dir(), "settings.toml")
	}
	if format == "" {
		format = SettingsFormatTOML
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure settings directory: %w", err)
	}

	var (
		data []byte
		err  error
	)

	switch format {
	case SettingsFormatTOML:
		data, err = toml.Marshal(settings)
	case SettingsFormatJSON:
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetIndent("", "  ")
		if err = encoder.Encode(settings); err == nil {
			data = buffer.Bytes()
		}
	default:
		return fmt.Errorf("unsupported settings format %q", format)
	}
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}

	if err := writeFileAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings %q: %w", path, err)
	}
	return nil
}

// write to temp file then rename so readers never see partial/corrupt data.
// rename is atomic on most filesystems so the settings file is always valid.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".resterm-settings-*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		closeErr := tmp.Close()
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}

	if err := tmp.Chmod(perm); err != nil {
		closeErr := tmp.Close()
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	return nil
}
