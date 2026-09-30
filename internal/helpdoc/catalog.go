// Package helpdoc provides the embedded documentation shown by the TUI.
package helpdoc

import (
	"embed"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

type Topic struct {
	ID      string
	Title   string
	Summary string
	Body    string
	Doc     DocRef

	meta string
	body string
}

// topicDef metadata is validated by the package tests, not at runtime.
// keywords associate loose editor words that are not worth an alias.
type topicDef struct {
	id       string
	title    string
	summary  string
	aliases  []string
	doc      DocRef
	keywords []string
}

//go:embed topics/*.md
var topicsFS embed.FS

var embedded = sync.OnceValue(func() catalog { return newCatalog(topicDefs(), topicsFS) })

type catalog struct {
	topics      []Topic
	byKey       map[string]int
	byDirective map[directive.Name]int
}

// Topics returns every topic in display order.
func Topics() []Topic {
	return slices.Clone(embedded().topics)
}

// Lookup resolves a topic ID, title, alias, keyword, or an @directive.
func Lookup(query string) (Topic, bool) {
	if name, ok := strings.CutPrefix(query, "@"); ok {
		spec, ok := directive.Lookup(directive.Name(strings.ToLower(name)))
		if !ok {
			return Topic{}, false
		}
		return Directive(spec.Name)
	}
	c := embedded()
	idx, ok := c.byKey[key(query)]
	if !ok {
		return Topic{}, false
	}
	return c.topics[idx], true
}

// Directive resolves a canonical directive name to its topic.
func Directive(name directive.Name) (Topic, bool) {
	c := embedded()
	idx, ok := c.byDirective[name]
	if !ok {
		return Topic{}, false
	}
	return c.topics[idx], true
}

// Search returns topics whose metadata or body contains every query token.
func Search(query string) []Topic {
	return find(query, true)
}

// Suggest matches metadata only, so completion results stay precise.
func Suggest(query string) []Topic {
	return find(query, false)
}

func find(query string, includeBody bool) []Topic {
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return Topics()
	}
	c := embedded()
	out := make([]Topic, 0, len(c.topics))
	for _, t := range c.topics {
		if t.matches(tokens, includeBody) {
			out = append(out, t)
		}
	}
	return out
}

func (t Topic) matches(tokens []string, includeBody bool) bool {
	for _, tok := range tokens {
		if strings.Contains(t.meta, tok) {
			continue
		}
		if includeBody && strings.Contains(t.body, tok) {
			continue
		}
		return false
	}
	return true
}

func newCatalog(defs []topicDef, files fs.FS) catalog {
	c := catalog{
		topics:      make([]Topic, 0, len(defs)),
		byKey:       make(map[string]int, len(defs)*3),
		byDirective: make(map[directive.Name]int, len(directive.Specs())),
	}
	for _, def := range defs {
		c.add(def, files)
	}
	for _, spec := range directive.Specs() {
		idx, ok := c.byKey[spec.Topic]
		if !ok {
			panic(fmt.Sprintf("helpdoc: directive %q names unknown topic %q", spec.Name, spec.Topic))
		}
		c.byDirective[spec.Name] = idx
	}
	return c
}

func (c *catalog) add(def topicDef, files fs.FS) {
	raw, err := fs.ReadFile(files, "topics/"+def.id+".md")
	if err != nil {
		panic("helpdoc: " + err.Error())
	}

	idx := len(c.topics)
	topic := Topic{
		ID:      def.id,
		Title:   def.title,
		Summary: def.summary,
		Body:    def.body(string(raw)),
		Doc:     def.doc,
	}
	topic.meta = strings.ToLower(
		def.id + " " + def.title + " " + def.summary + " " + strings.Join(def.aliases, " "),
	)
	topic.body = strings.ToLower(topic.Body)
	c.topics = append(c.topics, topic)

	keys := append([]string{def.id, def.title}, def.aliases...)
	for _, raw := range append(keys, def.keywords...) {
		k := key(raw)
		if prev, ok := c.byKey[k]; ok && prev != idx {
			panic(fmt.Sprintf("helpdoc: key %q belongs to both %q and %q", k, c.topics[prev].ID, def.id))
		}
		c.byKey[k] = idx
	}
}

// body strips the leading markdown H1 when it repeats the catalog title.
func (d topicDef) body(markdown string) string {
	markdown = strings.TrimSpace(markdown)
	first, rest, ok := strings.Cut(markdown, "\n")
	if ok && strings.TrimSpace(first) == "# "+d.title {
		return strings.TrimSpace(rest)
	}
	return markdown
}

func key(raw string) string {
	parts := strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		return unicode.IsSpace(r) || r == '_' || r == '-'
	})
	return strings.Join(parts, "-")
}

func topicDefs() []topicDef {
	return []topicDef{
		{
			id: "quick-start", title: "Quick Start",
			summary: "Create a request file and run the first request",
			aliases: []string{"start", "getting-started"},
			doc:     DocRef{Page: "quick-start"},
		},
		{
			id: "interface", title: "Interface",
			summary: "Understand the navigator, editor, response panes, and focus",
			aliases: []string{"ui", "layout"},
			doc:     DocRef{Page: "ui-tour"},
		},
		{
			id: "commands", title: "Commands and Shortcuts",
			summary:  "Use keys and Vim-style commands efficiently",
			aliases:  []string{"shortcuts", "keybindings", "command-line", "keys"},
			doc:      DocRef{Page: "ui-tour", Heading: "Core shortcuts"},
			keywords: []string{"command", "shortcut"},
		},
		{
			id: "requests", title: "Requests",
			summary: "Write request lines, metadata, headers, bodies, and separators",
			aliases: []string{"request-files"},
			doc:     DocRef{Page: "request-files"},
			keywords: []string{
				"get", "post", "put", "patch", "delete", "head", "options", "connect", "http",
			},
		},
		{
			id: "variables", title: "Variables and Environments",
			summary:  "Declare, select, interpolate, and capture values",
			aliases:  []string{"env", "environments", "interpolation"},
			doc:      DocRef{Page: "variables"},
			keywords: []string{"variable", "template"},
		},
		{
			id: "authentication", title: "Authentication",
			summary: "Configure static, OAuth, command-backed, and captured credentials",
			aliases: []string{"auth", "oauth", "oauth2"},
			doc:     DocRef{Page: "authentication"},
		},
		{
			id: "transport", title: "HTTP Transport and Settings",
			summary:  "Control timeouts, TLS, proxies, redirects, and HTTP behavior",
			aliases:  []string{"settings", "tls", "proxy"},
			doc:      DocRef{Page: "http-settings"},
			keywords: []string{"https", "timeout"},
		},
		{
			id: "ssh", title: "SSH Tunnels",
			summary: "Route requests through an SSH jump host",
			doc:     DocRef{Page: "ssh-tunnels"},
		},
		{
			id: "kubernetes", title: "Kubernetes Port-Forwards",
			summary: "Reach pods and workloads through managed port-forwards",
			aliases: []string{"k8s", "port-forward"},
			doc:     DocRef{Page: "kubernetes"},
		},
		{
			id: "mocks", title: "Mock Servers",
			summary: "Define mock routes, sequences, matches, and expectations",
			aliases: []string{"mock", "mock-server"},
			doc:     DocRef{Page: "mock-servers"},
		},
		{
			id: "recording", title: "Recording Traffic",
			summary: "Record HTTP traffic as requests or mocks",
			aliases: []string{"record", "recorder"},
			doc:     DocRef{Page: "recording"},
		},
		{
			id: "workflows", title: "Workflows",
			summary: "Chain named requests with conditions, branches, and loops",
			aliases: []string{"workflow"},
			doc:     DocRef{Page: "workflows"},
		},
		{
			id: "rts", title: "RestermScript",
			summary: "Use expression directives, modules, patches, and assertions",
			aliases: []string{"restermscript"},
			doc:     DocRef{Page: "rts"},
		},
		{
			id: "scripting", title: "Scripting API",
			summary: "Run JavaScript before requests and in response tests",
			aliases: []string{"script", "javascript"},
			doc:     DocRef{Page: "scripting"},
		},
		{
			id: "streaming", title: "Streaming",
			summary: "Work with Server-Sent Events and WebSocket sessions",
			aliases: []string{"sse", "websocket", "websockets", "ws"},
			doc:     DocRef{Page: "streaming"},
		},
		{
			id: "graphql", title: "GraphQL",
			summary: "Select operations and provide queries and variables",
			aliases: []string{"gql"},
			doc:     DocRef{Page: "graphql"},
		},
		{
			id: "grpc", title: "gRPC",
			summary: "Call unary or streaming RPC methods with metadata",
			aliases: []string{"rpc"},
			doc:     DocRef{Page: "grpc"},
		},
		{
			id: "tracing", title: "Timeline and Tracing",
			summary: "Inspect request phases and set latency budgets",
			aliases: []string{"trace", "timeline"},
			doc:     DocRef{Page: "ui-tour", Heading: "Timeline & tracing"},
		},
		{
			id: "profiling", title: "Profiling Requests",
			summary: "Repeat requests and inspect latency distributions",
			aliases: []string{"profile", "benchmark"},
			doc:     DocRef{Page: "profiling"},
		},
		{
			id: "polling", title: "Polling and Retries",
			summary:  "Retry failed requests or poll until a response matches",
			aliases:  []string{"poll", "retry", "retries", "backoff"},
			doc:      DocRef{Page: "polling-and-retries"},
			keywords: []string{"retry-after", "long-running jobs"},
		},
		{
			id: "comparison", title: "Compare Runs",
			summary: "Run one request across multiple environments",
			aliases: []string{"compare", "compare-runs"},
			doc:     DocRef{Page: "compare-runs"},
		},
		{
			id: "history", title: "Response History and Diffing",
			summary: "Review, replay, compare, and delete saved responses",
			aliases: []string{"response-history", "diff", "diffing"},
			doc:     DocRef{Page: "history"},
		},
		{
			id: "configuration", title: "Configuration",
			summary:  "Customize settings, bindings, themes, and layout",
			aliases:  []string{"config", "bindings", "themes", "theming"},
			doc:      DocRef{Page: "configuration"},
			keywords: []string{"binding", "theme"},
		},
		{
			id: "cli", title: "CLI Reference",
			summary: "Run requests, mocks, imports, and history outside the TUI",
			aliases: []string{"command-line-interface"},
			doc:     DocRef{Page: "cli"},
		},
	}
}
