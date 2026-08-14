package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/helpdoc"
	"github.com/unkn0wn-root/resterm/internal/prompt"
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

// anyArgs is the maxArgs of a subcommand that parses its own flags.
const anyArgs = -1

// args spells every flag out for the usage line. hint is the shorter form the
// picker shows. Only commands whose full grammar would crowd the summary out of
// the row need both.
type mockCommandDef struct {
	name    string
	args    string
	hint    string
	summary string
	maxArgs int
}

func (d mockCommandDef) acceptsArgs() bool { return d.maxArgs != 0 }

func (d mockCommandDef) tooManyArgs(n int) bool { return d.maxArgs != anyArgs && n > d.maxArgs }

type exCatalog struct {
	defs []exCommandDef
	mock []mockCommandDef
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
		{
			kind: exCommandEdit, name: "edit", aliases: []string{"e"},
			usage: "edit [path]", summary: "Open a file or workspace", hasArgs: true,
		},
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
		{
			name: "start", args: "[host:port] [--source files] [--recursive] [--all]",
			hint: "[host:port] [flags]", summary: "Start the mock server", maxArgs: anyArgs,
		},
		{name: "stop", summary: "Stop the mock server"},
		{
			name: "restart", args: "[host:port] [--source files] [--recursive] [--all]",
			hint: "[host:port] [flags]", summary: "Restart with another address or scope", maxArgs: anyArgs,
		},
		{name: "logs", summary: "Open the request log"},
		{name: "clear", summary: "Clear request logs and verification journal"},
		{name: "reset", args: "[sequence]", summary: "Reset all or one response sequence", maxArgs: 1},
		{name: "verify", summary: "Check active @expect declarations"},
		{name: "capture", summary: "Capture the focused response as a mock"},
	},
}

func (d exCommandDef) label() string {
	if d.usage != "" {
		return d.usage
	}
	return d.name
}

// label goes in the picker, where it shares the row with the summary. usage is
// the full grammar, for the usage line and the argument hint.
func (d mockCommandDef) label() string { return joinArgs(d.name, cmp.Or(d.hint, d.args)) }

func (d mockCommandDef) usage() string { return joinArgs(d.name, d.args) }

func joinArgs(name, args string) string {
	if args == "" {
		return name
	}
	return name + " " + args
}

func (c exCatalog) Parse(input string) exCommand {
	line := prompt.Lex(input)
	if line.Unclosed != 0 {
		return exCommand{kind: exCommandInvalid, err: fmt.Errorf("unclosed %q quote", line.Unclosed)}
	}
	fields := line.Values()
	if len(fields) == 0 {
		return exCommand{kind: exCommandEmpty}
	}

	bang := strings.HasSuffix(fields[0], "!")
	def, ok := c.Lookup(commandName(fields[0]))
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

func commandName(token string) string {
	return strings.ToLower(strings.TrimSuffix(token, "!"))
}

type lineBody struct {
	text  string
	start int
	end   int
}

func commandBody(input string) lineBody {
	text, start, end := prompt.Body(input)
	return lineBody{text: text, start: start, end: end}
}

func (b lineBody) item(label, summary, replacement string) prompt.Item {
	return prompt.Item{
		Label:   label,
		Summary: summary,
		Edit:    prompt.Edit{Start: b.start, End: b.end, Text: replacement},
	}
}

func (c exCatalog) Suggestions(input string) []prompt.Item {
	body := commandBody(input)
	head, rest, typed := cutSpace(body.text)
	if !typed {
		return c.commandSuggestions(body)
	}

	def, ok := c.Lookup(commandName(head))
	if !ok {
		return nil
	}
	switch def.kind {
	case exCommandHelp, exCommandDocs:
		return topicSuggestions(body, def.name, rest)
	case exCommandMock:
		return c.mockSuggestions(body, rest)
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

func (c exCatalog) commandSuggestions(body lineBody) []prompt.Item {
	filter := commandName(strings.TrimSpace(body.text))
	out := make([]prompt.Item, 0, len(c.defs))
	for _, def := range c.defs {
		if filter != "" && !def.matches(filter) {
			continue
		}
		insert := def.name
		if def.hasArgs {
			insert += " "
		}
		out = append(out, body.item(def.label(), def.summary, insert))
	}
	return out
}

func topicSuggestions(body lineBody, command, filter string) []prompt.Item {
	topics := helpdoc.Suggest(filter)
	out := make([]prompt.Item, 0, len(topics))
	for _, topic := range topics {
		out = append(out, body.item(topic.ID, topic.Summary, command+" "+topic.ID))
	}
	return out
}

// mockSuggestions lists the subcommands until one is named. After that the list
// has nothing left to offer, so it gives way to the grammar of the named
// subcommand. That hint inserts the line unchanged, so completing it leaves
// what was typed alone.
func (c exCatalog) mockSuggestions(body lineBody, rest string) []prompt.Item {
	head, _, typing := cutSpace(rest)
	name := strings.ToLower(head)
	if typing {
		def, ok := c.Mock(name)
		if !ok || !def.acceptsArgs() {
			return nil
		}
		return []prompt.Item{body.item(def.usage(), "", body.text)}
	}

	out := make([]prompt.Item, 0, len(c.mock))
	for _, def := range c.mock {
		if name != "" && !strings.Contains(def.name, name) {
			continue
		}
		insert := "mock " + def.name
		if def.acceptsArgs() {
			insert += " "
		}
		out = append(out, body.item(def.label(), def.summary, insert))
	}
	return out
}

// cutSpace splits the first word off s. Anything after that word counts,
// including a lone trailing space.
func cutSpace(s string) (head, rest string, found bool) {
	idx := strings.IndexFunc(s, unicode.IsSpace)
	if idx < 0 {
		return s, "", false
	}
	return s[:idx], s[idx+1:], true
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
