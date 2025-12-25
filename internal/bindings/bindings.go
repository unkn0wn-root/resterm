package bindings

import (
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

// Format identifies the serialization format for shortcut configs.
type Format string

const (
	FormatTOML Format = "toml"
	FormatJSON Format = "json"
)

// Source describes where the bindings config was loaded from.
type Source struct {
	Path   string
	Format Format
}

// ActionID uniquely identifies a shortcut action.
type ActionID string

// Binding represents a resolved shortcut binding.
type Binding struct {
	Action     ActionID
	Steps      []string
	Repeatable bool
}

// Map stores runtime shortcut bindings and lookup helpers.
type Map struct {
	single        map[string]bindingRef
	chords        map[string]map[string]bindingRef
	chordPrefixes map[string]struct{}
	actions       map[ActionID]*actionEntry
}

type bindingRef struct {
	action     ActionID
	steps      []string
	repeatable bool
}

type actionEntry struct {
	repeatable bool
	bindings   []bindingRef
}

// Load attempts to read bindings from bindings.toml/json in dir. Missing files fall back to defaults.
func Load(dir string) (*Map, Source, error) {
	candidates := []Source{
		{Path: filepath.Join(dir, "bindings.toml"), Format: FormatTOML},
		{Path: filepath.Join(dir, "bindings.json"), Format: FormatJSON},
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
				fmt.Errorf("read bindings %q: %w", candidate.Path, err),
			)
			continue
		}

		overrides, err := parseConfig(data, candidate.Format)
		if err != nil {
			return nil, Source{}, fmt.Errorf("parse bindings %q: %w", candidate.Path, err)
		}
		built, err := buildMap(overrides)
		if err != nil {
			return nil, Source{}, fmt.Errorf("apply bindings %q: %w", candidate.Path, err)
		}
		return built, candidate, nil
	}

	if accumulated != nil {
		return nil, Source{}, accumulated
	}

	built, err := buildMap(nil)
	if err != nil {
		return nil, Source{}, err
	}
	return built, Source{Path: candidates[0].Path, Format: FormatTOML}, nil
}

// DefaultMap builds the built-in bindings without consulting disk.
func DefaultMap() *Map {
	m, err := buildMap(nil)
	if err != nil {
		panic(err)
	}
	return m
}

// MatchSingle returns the binding bound to a single-step shortcut, if any.
func (m *Map) MatchSingle(key string) (Binding, bool) {
	if m == nil {
		return Binding{}, false
	}
	ref, ok := m.single[key]
	if !ok {
		return Binding{}, false
	}
	return ref.binding(), true
}

// HasChordPrefix reports whether the given key can start a chord sequence.
func (m *Map) HasChordPrefix(key string) bool {
	if m == nil {
		return false
	}
	_, ok := m.chordPrefixes[key]
	return ok
}

// ResolveChord resolves a chord prefix + next key into a binding.
func (m *Map) ResolveChord(prefix, next string) (Binding, bool) {
	if m == nil {
		return Binding{}, false
	}
	nextMap, ok := m.chords[prefix]
	if !ok {
		return Binding{}, false
	}
	ref, ok := nextMap[next]
	if !ok {
		return Binding{}, false
	}
	return ref.binding(), true
}

// Bindings returns a copy of every binding for the provided action.
func (m *Map) Bindings(action ActionID) []Binding {
	if m == nil {
		return nil
	}
	entry, ok := m.actions[action]
	if !ok || len(entry.bindings) == 0 {
		return nil
	}
	out := make([]Binding, 0, len(entry.bindings))
	for _, ref := range entry.bindings {
		out = append(out, ref.binding())
	}
	return out
}

func (ref bindingRef) binding() Binding {
	seq := make([]string, len(ref.steps))
	copy(seq, ref.steps)
	repeat := ref.repeatable && len(seq) > 1
	return Binding{Action: ref.action, Steps: seq, Repeatable: repeat}
}

type configFile struct {
	Bindings map[string][]string `json:"bindings" toml:"bindings"`
}

func parseConfig(data []byte, format Format) (map[ActionID][][]string, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var payload configFile
	switch format {
	case FormatTOML:
		if err := toml.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
	case FormatJSON:
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}

	if len(payload.Bindings) == 0 {
		return nil, nil
	}

	overrides := make(map[ActionID][][]string, len(payload.Bindings))
	for key, specs := range payload.Bindings {
		id := ActionID(key)
		def, ok := definitionLookup[id]
		if !ok {
			return nil, fmt.Errorf("unknown action %q", key)
		}
		sequences := make([][]string, 0, len(specs))
		for _, spec := range specs {
			seq, err := parseSequence(spec)
			if err != nil {
				return nil, fmt.Errorf("action %q: %w", key, err)
			}
			sequences = append(sequences, seq)
		}
		overrides[def.id] = sequences
	}
	return overrides, nil
}

func buildMap(overrides map[ActionID][][]string) (*Map, error) {
	bindingsByAction := make(map[ActionID][][]string, len(definitions))
	repeatableByAction := make(map[ActionID]bool, len(definitions))
	for _, def := range definitions {
		repeatableByAction[def.id] = def.repeatable
		defaults := make([][]string, len(def.defaults))
		for i, seq := range def.defaults {
			cp := make([]string, len(seq))
			copy(cp, seq)
			defaults[i] = cp
		}
		bindingsByAction[def.id] = defaults
	}

	for id, seqs := range overrides {
		// copy to avoid retaining slices backed by decode buffer
		dup := make([][]string, len(seqs))
		for i, seq := range seqs {
			cp := make([]string, len(seq))
			copy(cp, seq)
			dup[i] = cp
		}
		bindingsByAction[id] = dup
	}

	single := make(map[string]bindingRef)
	chords := make(map[string]map[string]bindingRef)
	chordPrefixes := make(map[string]struct{})
	actions := make(map[ActionID]*actionEntry, len(definitions))

	for id, seqs := range bindingsByAction {
		repeatable := repeatableByAction[id]
		entry := &actionEntry{repeatable: repeatable}
		actions[id] = entry
		seen := make(map[string]struct{})
		for _, seq := range seqs {
			if len(seq) == 0 {
				continue
			}
			if len(seq) > 2 {
				return nil, fmt.Errorf("action %s: bindings may not exceed two steps", id)
			}
			if id == ActionSendRequest && len(seq) != 1 {
				return nil, fmt.Errorf("action %s only supports single-step bindings", id)
			}
			key := strings.Join(seq, " ⇒ ")
			if _, ok := seen[key]; ok {
				return nil, fmt.Errorf(
					"action %s: duplicate binding %q",
					id,
					strings.Join(seq, " "),
				)
			}
			seen[key] = struct{}{}

			ref := bindingRef{
				action:     id,
				steps:      append([]string(nil), seq...),
				repeatable: repeatable,
			}
			entry.bindings = append(entry.bindings, ref)

			if len(seq) == 1 {
				step := seq[0]
				if existing, ok := single[step]; ok {
					return nil, fmt.Errorf(
						"binding %q assigned to both %s and %s",
						step,
						existing.action,
						id,
					)
				}
				single[step] = ref
				continue
			}

			prefix := seq[0]
			next := seq[1]
			bucket := chords[prefix]
			if bucket == nil {
				bucket = make(map[string]bindingRef)
				chords[prefix] = bucket
			}
			if existing, ok := bucket[next]; ok {
				return nil, fmt.Errorf(
					"binding %q %q assigned to both %s and %s",
					prefix,
					next,
					existing.action,
					id,
				)
			}
			bucket[next] = ref
			chordPrefixes[prefix] = struct{}{}
		}
	}

	for prefix := range chordPrefixes {
		if existing, ok := single[prefix]; ok {
			return nil, fmt.Errorf(
				"key %q cannot be both a chord prefix and standalone shortcut (conflicts with %s)",
				prefix,
				existing.action,
			)
		}
	}

	return &Map{
		single:        single,
		chords:        chords,
		chordPrefixes: chordPrefixes,
		actions:       actions,
	}, nil
}

func parseSequence(spec string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, errors.New("empty binding")
	}
	parts := strings.Fields(spec)
	if len(parts) == 0 {
		return nil, errors.New("empty binding")
	}
	out := make([]string, len(parts))
	for i, part := range parts {
		normalized, err := normalizeStep(part)
		if err != nil {
			return nil, err
		}
		out[i] = normalized
	}
	return out, nil
}

func normalizeStep(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty key step")
	}
	if raw == "?" {
		raw = "shift+/"
	}
	if raw == " " {
		raw = "space"
	}

	runes := []rune(raw)
	if len(runes) == 1 && !strings.Contains(raw, "+") {
		r := runes[0]
		if unicode.IsLetter(r) {
			if unicode.IsUpper(r) {
				return "shift+" + strings.ToLower(raw), nil
			}
			return strings.ToLower(raw), nil
		}
		return strings.ToLower(raw), nil
	}

	if !strings.Contains(raw, "+") {
		return strings.ToLower(raw), nil
	}

	parts := strings.Split(raw, "+")
	var keyParts []string
	modSet := make(map[string]struct{})
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		switch lower {
		case "ctrl", "control":
			modSet["ctrl"] = struct{}{}
		case "alt", "option":
			modSet["alt"] = struct{}{}
		case "shift":
			modSet["shift"] = struct{}{}
		case "cmd", "command", "meta":
			modSet["cmd"] = struct{}{}
		default:
			keyParts = append(keyParts, lower)
		}
	}
	if len(keyParts) == 0 {
		return "", fmt.Errorf("binding %q missing key", raw)
	}
	key := strings.Join(keyParts, "+")
	mods := orderedModifiers(modSet)
	if len(mods) == 0 {
		return key, nil
	}
	return strings.Join(append(mods, key), "+"), nil
}

func orderedModifiers(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	order := []string{"ctrl", "alt", "shift", "cmd"}
	out := make([]string, 0, len(set))
	for _, mod := range order {
		if _, ok := set[mod]; ok {
			out = append(out, mod)
		}
	}
	return out
}

// NormalizeKeyString converts runtime key strings into canonical form for lookup.
func NormalizeKeyString(raw string) string {
	normalized, err := normalizeStep(raw)
	if err != nil {
		return ""
	}
	return normalized
}

func actionIDs() []ActionID {
	ids := make([]ActionID, 0, len(definitions))
	for _, def := range definitions {
		ids = append(ids, def.id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// KnownActions returns the sorted list of action identifiers.
func KnownActions() []ActionID {
	return actionIDs()
}
