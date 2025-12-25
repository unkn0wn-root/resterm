package theme

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	toml "github.com/pelletier/go-toml/v2"
)

type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
)

type Format string

const (
	FormatBuiltin Format = "builtin"
	FormatJSON    Format = "json"
	FormatTOML    Format = "toml"
)

type Definition struct {
	Key         string
	DisplayName string
	Metadata    Metadata
	Theme       Theme
	Source      Source
	Format      Format
	Path        string
}

type Catalog struct {
	order []Definition
	index map[string]int
}

func (c Catalog) All() []Definition {
	out := make([]Definition, len(c.order))
	copy(out, c.order)
	return out
}

func (c Catalog) Keys() []string {
	keys := make([]string, len(c.order))
	for i, def := range c.order {
		keys[i] = def.Key
	}
	return keys
}

func (c Catalog) Get(key string) (Definition, bool) {
	if c.index == nil {
		return Definition{}, false
	}
	idx, ok := c.index[key]
	if !ok {
		return Definition{}, false
	}
	return c.order[idx], true
}

func (c *Catalog) add(def Definition) {
	if c.index == nil {
		c.index = make(map[string]int)
	}
	c.index[def.Key] = len(c.order)
	c.order = append(c.order, def)
}

func LoadCatalog(dirs []string) (Catalog, error) {
	base := DefaultTheme()
	defs := make([]Definition, 0, 1)
	usedKeys := map[string]int{"default": 1}

	defs = append(defs, Definition{
		Key:         "default",
		DisplayName: "Default",
		Metadata: Metadata{
			Name: "Default",
		},
		Theme:  base,
		Source: SourceBuiltin,
		Format: FormatBuiltin,
		Path:   "",
	})

	var combinedErr error

	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			combinedErr = errors.Join(
				combinedErr,
				fmt.Errorf("themes: read directory %q: %w", dir, err),
			)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			var format Format
			switch ext {
			case ".toml":
				format = FormatTOML
			case ".json":
				format = FormatJSON
			default:
				continue
			}
			path := filepath.Join(dir, entry.Name())
			def, err := loadUserTheme(path, format, base)
			if err != nil {
				combinedErr = errors.Join(combinedErr, fmt.Errorf("themes: load %q: %w", path, err))
				continue
			}
			keyCandidate := def.Key
			def.Key = ensureUniqueKey(keyCandidate, usedKeys)
			if strings.TrimSpace(def.DisplayName) == "" {
				def.DisplayName = humaniseSlug(def.Key)
			}
			defs = append(defs, def)
		}
	}

	catalog := assembleCatalog(defs)
	if combinedErr != nil {
		return catalog, combinedErr
	}
	return catalog, nil
}

func loadUserTheme(path string, format Format, base Theme) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}
	spec, err := decodeThemeSpec(data, format)
	if err != nil {
		return Definition{}, err
	}
	theme, err := ApplySpec(base, spec)
	if err != nil {
		return Definition{}, err
	}

	meta := Metadata{}
	if spec.Metadata != nil {
		meta = *spec.Metadata
	}
	displayName := strings.TrimSpace(meta.Name)
	slug := slugify(meta.Name)
	if slug == "" {
		baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		slug = slugify(baseName)
	}
	def := Definition{
		Key:         slug,
		DisplayName: displayName,
		Metadata:    meta,
		Theme:       theme,
		Source:      SourceUser,
		Format:      format,
		Path:        path,
	}
	return def, nil
}

func decodeThemeSpec(data []byte, format Format) (ThemeSpec, error) {
	var spec ThemeSpec
	switch format {
	case FormatJSON:
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&spec); err != nil {
			return ThemeSpec{}, err
		}
	case FormatTOML:
		if err := toml.Unmarshal(data, &spec); err != nil {
			return ThemeSpec{}, err
		}
	default:
		return ThemeSpec{}, fmt.Errorf("decode: unsupported format %q", format)
	}
	return spec, nil
}

func assembleCatalog(defs []Definition) Catalog {
	var catalog Catalog
	if len(defs) == 0 {
		return catalog
	}
	catalog.add(defs[0])
	if len(defs) == 1 {
		return catalog
	}
	custom := make([]Definition, len(defs)-1)
	copy(custom, defs[1:])
	sort.SliceStable(custom, func(i, j int) bool {
		left := strings.ToLower(custom[i].DisplayName)
		right := strings.ToLower(custom[j].DisplayName)
		if left == right {
			return custom[i].Key < custom[j].Key
		}
		return left < right
	})
	for _, def := range custom {
		catalog.add(def)
	}
	return catalog
}

func ensureUniqueKey(candidate string, used map[string]int) string {
	key := candidate
	if strings.TrimSpace(key) == "" {
		key = "theme"
	}
	base := key
	counter := used[base]
	if counter == 0 {
		used[base] = 1
		used[key] = 1
		return key
	}
	for {
		suffix := fmt.Sprintf("%s-%d", base, counter)
		if _, exists := used[suffix]; !exists {
			used[base] = counter + 1
			used[suffix] = 1
			return suffix
		}
		counter++
	}
}

func slugify(name string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || unicode.IsSpace(r):
			if !lastDash {
				builder.WriteRune('-')
				lastDash = true
			}
		}
	}
	slug := builder.String()
	slug = strings.Trim(slug, "-")
	return slug
}

func humaniseSlug(slug string) string {
	if slug == "" {
		return "Theme"
	}
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}
