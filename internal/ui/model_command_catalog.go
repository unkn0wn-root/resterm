package ui

import (
	"slices"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/helpdoc"
)

// usage is only set when it adds argument hints beyond the plain name.
type exCommandDef struct {
	kind    exCommandKind
	name    string
	aliases []string
	usage   string
	summary string
	hasArgs bool
	noBang  bool
}

type mockCommandDef struct {
	name    string
	usage   string
	summary string
	maxArgs int
}

type exCatalog struct {
	defs []exCommandDef
	mock []mockCommandDef
}

type exSuggestion struct {
	label   string
	summary string
	insert  string
}

type exSuggestionState struct {
	items     []exSuggestion
	selection int
}

var exCommands = exCatalog{
	defs: []exCommandDef{
		{kind: exCommandWrite, name: "write", aliases: []string{"w"}, summary: "Save the current file"},
		{
			kind: exCommandQuit, name: "quit", aliases: []string{"q", "qa", "qall"},
			usage: "quit[!]", summary: "Quit; ! discards unsaved changes",
		},
		{kind: exCommandWriteQuit, name: "wq", summary: "Save and quit"},
		{
			kind: exCommandExit, name: "exit", aliases: []string{"x", "xit"},
			summary: "Save changes when needed, then quit",
		},
		{kind: exCommandEdit, name: "edit", aliases: []string{"e"}, summary: "Open the file or folder prompt"},
		{
			kind: exCommandHelp, name: "help", aliases: []string{"h", "man"},
			usage: "help [topic]", summary: "Open embedded help", hasArgs: true,
		},
		{kind: exCommandNoHighlight, name: "nohlsearch", aliases: []string{"noh"}, summary: "Clear search highlights"},
		{
			kind: exCommandMock, name: "mock",
			usage: "mock [command]", summary: "Control the workspace mock server", hasArgs: true, noBang: true,
		},
		{
			kind: exCommandDocs, name: "docs",
			usage: "docs [topic]", summary: "Open version-matched web documentation", hasArgs: true, noBang: true,
		},
	},
	mock: []mockCommandDef{
		{name: "status", summary: "Show server address and counters"},
		{name: "start", usage: "start [host:port]", summary: "Start the mock server", maxArgs: 1},
		{name: "stop", summary: "Stop the mock server"},
		{name: "restart", usage: "restart [host:port]", summary: "Restart, optionally on another address", maxArgs: 1},
		{name: "logs", summary: "Open the request log"},
		{name: "clear", summary: "Clear request logs and verification journal"},
		{name: "reset", usage: "reset [sequence]", summary: "Reset all or one response sequence", maxArgs: 1},
		{name: "verify", summary: "Check active @expect declarations"},
		{name: "capture", summary: "Capture the focused response as a mock"},
	},
}

func (d exCommandDef) display() string {
	if d.usage != "" {
		return d.usage
	}
	return d.name
}

func (d mockCommandDef) display() string {
	if d.usage != "" {
		return d.usage
	}
	return d.name
}

func (c exCatalog) Parse(input string) exCommand {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(input), ":"))
	if len(fields) == 0 {
		return exCommand{kind: exCommandEmpty}
	}

	rawName := fields[0]
	bang := strings.HasSuffix(rawName, "!")
	name := strings.ToLower(strings.TrimSuffix(rawName, "!"))
	def, ok := c.Lookup(name)
	if !ok || (bang && def.noBang) {
		return exCommand{kind: exCommandUnknown, name: strings.Join(fields, " ")}
	}
	if !def.hasArgs && len(fields) > 1 {
		return exCommand{kind: exCommandTrailing, name: strings.Join(fields[1:], " ")}
	}
	return exCommand{kind: def.kind, bang: bang, args: fields[1:]}
}

// Lookup expects a lowercase name without its bang suffix.
func (c exCatalog) Lookup(name string) (exCommandDef, bool) {
	for _, def := range c.defs {
		if name == def.name || slices.Contains(def.aliases, name) {
			return def, true
		}
	}
	return exCommandDef{}, false
}

func (c exCatalog) Suggestions(input string) []exSuggestion {
	body := strings.TrimPrefix(strings.TrimLeftFunc(input, unicode.IsSpace), ":")
	idx := strings.IndexFunc(body, unicode.IsSpace)
	if idx < 0 {
		return c.commandSuggestions(body)
	}

	def, ok := c.Lookup(strings.ToLower(strings.TrimSuffix(body[:idx], "!")))
	if !ok {
		return nil
	}
	rest := body[idx+1:]
	switch def.kind {
	case exCommandHelp, exCommandDocs:
		return topicSuggestions(def.name, rest)
	case exCommandMock:
		return c.mockSuggestions(rest)
	default:
		return nil
	}
}

// Mock expects a lowercase subcommand name.
func (c exCatalog) Mock(name string) (mockCommandDef, bool) {
	for _, def := range c.mock {
		if def.name == name {
			return def, true
		}
	}
	return mockCommandDef{}, false
}

func (c exCatalog) mockNames() []string {
	names := make([]string, len(c.mock))
	for i, def := range c.mock {
		names[i] = def.name
	}
	return names
}

func (c exCatalog) commandSuggestions(filter string) []exSuggestion {
	filter = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(filter), "!"))
	out := make([]exSuggestion, 0, len(c.defs))
	for _, def := range c.defs {
		if filter != "" && !def.matches(filter) {
			continue
		}
		insert := def.name
		if def.hasArgs {
			insert += " "
		}
		out = append(out, exSuggestion{label: def.display(), summary: def.summary, insert: insert})
	}
	return out
}

func topicSuggestions(command, filter string) []exSuggestion {
	topics := helpdoc.Suggest(filter)
	out := make([]exSuggestion, 0, len(topics))
	for _, topic := range topics {
		out = append(out, exSuggestion{
			label:   topic.ID,
			summary: topic.Summary,
			insert:  command + " " + topic.ID,
		})
	}
	return out
}

func (c exCatalog) mockSuggestions(rest string) []exSuggestion {
	name := strings.ToLower(strings.TrimSpace(rest))
	if strings.ContainsFunc(name, unicode.IsSpace) {
		return nil
	}
	// a complete name followed by a space means the user moved on to its argument
	if len(rest) > len(name) {
		if _, ok := c.Mock(name); ok {
			return nil
		}
	}

	out := make([]exSuggestion, 0, len(c.mock))
	for _, def := range c.mock {
		if name != "" && !strings.Contains(def.name, name) {
			continue
		}
		insert := "mock " + def.name
		if def.maxArgs > 0 {
			insert += " "
		}
		out = append(out, exSuggestion{label: def.display(), summary: def.summary, insert: insert})
	}
	return out
}

func (d exCommandDef) matches(filter string) bool {
	if strings.Contains(d.name, filter) {
		return true
	}
	for _, alias := range d.aliases {
		if strings.Contains(alias, filter) {
			return true
		}
	}
	return false
}

func (s *exSuggestionState) reset(items []exSuggestion) {
	s.items = items
	s.selection = -1
}

func (s *exSuggestionState) move(delta int) {
	if len(s.items) == 0 {
		return
	}
	if s.selection < 0 {
		if delta < 0 {
			s.selection = len(s.items) - 1
		} else {
			s.selection = 0
		}
		return
	}
	idx := (s.selection + delta) % len(s.items)
	if idx < 0 {
		idx += len(s.items)
	}
	s.selection = idx
}

func (s exSuggestionState) selected() (exSuggestion, bool) {
	if s.selection < 0 {
		return exSuggestion{}, false
	}
	return s.items[s.selection], true
}

func (s exSuggestionState) completion() (exSuggestion, bool) {
	if item, ok := s.selected(); ok {
		return item, true
	}
	if len(s.items) == 0 {
		return exSuggestion{}, false
	}
	return s.items[0], true
}

func (s exSuggestionState) display(limit int) ([]exSuggestion, int, bool) {
	if len(s.items) == 0 || limit <= 0 {
		return nil, 0, false
	}
	start, end := popupWindow(s.selection, limit, len(s.items))
	selection := -1
	if s.selection >= 0 {
		selection = s.selection - start
	}
	return s.items[start:end], selection, true
}
