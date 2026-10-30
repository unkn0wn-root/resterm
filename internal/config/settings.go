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

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/fsx"
)

const (
	SettingsFormatTOML SettingsFormat = "toml"
	SettingsFormatJSON SettingsFormat = "json"
)

type Settings struct {
	DefaultTheme string            `json:"default_theme"      toml:"default_theme"`
	Layout       LayoutSettings    `json:"layout"             toml:"layout"`
	Editor       EditorSettings    `json:"editor,omitzero"    toml:"editor,omitempty"`
	Clipboard    ClipboardSettings `json:"clipboard,omitzero" toml:"clipboard,omitempty"`
}

type EditorSettings struct {
	Diagnostics *bool `json:"diagnostics,omitempty" toml:"diagnostics,omitempty"`
}

func (s EditorSettings) DiagnosticsEnabled() bool {
	return s.Diagnostics == nil || *s.Diagnostics
}

type ClipboardSettings struct {
	OSC52Limit *ByteSize `json:"osc52_limit,omitempty" toml:"osc52_limit,omitempty"`
}

func (s ClipboardSettings) OSC52LimitBytes() int64 {
	if s.OSC52Limit == nil {
		return 0
	}
	return s.OSC52Limit.n
}

// ByteSize keeps the text it was parsed from, so a save writes back what the user typed.
type ByteSize struct {
	text string
	n    int64
}

func (b ByteSize) MarshalText() ([]byte, error) {
	return []byte(b.text), nil
}

func (b *ByteSize) UnmarshalText(text []byte) error {
	n, err := bytesize.Parse(string(text))
	if err != nil {
		return err
	}
	if n <= 0 {
		return fmt.Errorf("invalid size %q: must be greater than zero", text)
	}
	*b = ByteSize{text: string(text), n: n}
	return nil
}

type SettingsFormat string
type SettingsHandle struct {
	Path   string
	Format SettingsFormat
	err    error
}

// tries loading TOML first, then JSON, then returns default settings and the TOML handle if neither exists.
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
			err = fmt.Errorf("parse settings %q: %w", candidate.Path, err)
			return Settings{}, SettingsHandle{err: err}, err
		}
		settings.Layout = NormaliseLayoutSettings(settings.Layout)
		return settings, candidate, nil
	}

	if accumulated != nil {
		return Settings{}, SettingsHandle{err: accumulated}, accumulated
	}

	return Settings{Layout: DefaultLayoutSettings()}, candidates[0], nil
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
	if handle.err != nil {
		return fmt.Errorf("settings not saved: %w", handle.err)
	}
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

	if err := fsx.WriteAtomic(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings %q: %w", path, err)
	}
	return nil
}
