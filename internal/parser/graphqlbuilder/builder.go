package graphqlbuilder

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type Builder struct {
	enabled          bool
	operation        string
	collectVariables bool
	variablesLines   []string
	variablesFile    string
	queryLines       []string
	queryFile        string
}

func New() *Builder {
	return &Builder{}
}

func (b *Builder) HandleDirective(key, rest string) bool {
	switch key {
	case "graphql":
		if rest == "" || strings.EqualFold(rest, "true") {
			b.enabled = true
			return true
		} else if strings.EqualFold(rest, "false") {
			b.disable()
			return true
		}
		return true
	case "operation", "graphql-operation":
		if b.enabled {
			b.operation = rest
		}
		return b.enabled
	case "variables":
		if !b.enabled {
			return false
		}
		b.collectVariables = true
		b.variablesLines = nil
		b.variablesFile = ""

		rest = strings.TrimSpace(rest)
		if rest != "" {
			if strings.HasPrefix(rest, "<") {
				b.variablesFile = strings.TrimSpace(strings.TrimPrefix(rest, "<"))
			} else {
				b.variablesLines = append(b.variablesLines, rest)
			}
		}
		return true
	case "query":
		if !b.enabled {
			return false
		}
		b.collectVariables = false
		b.queryLines = nil
		b.queryFile = ""

		rest = strings.TrimSpace(rest)
		if rest != "" {
			if strings.HasPrefix(rest, "<") {
				b.queryFile = strings.TrimSpace(strings.TrimPrefix(rest, "<"))
				return true
			}
			b.queryLines = append(b.queryLines, rest)
		}
		return true
	}
	return false
}

func (b *Builder) disable() {
	b.enabled = false
	b.operation = ""
	b.collectVariables = false
	b.variablesLines = nil
	b.variablesFile = ""
	b.queryLines = nil
	b.queryFile = ""
}

func (b *Builder) Enabled() bool {
	return b.enabled
}

func (b *Builder) HandleBodyLine(line string) bool {
	if !b.enabled {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if b.collectVariables {
		if strings.HasPrefix(trimmed, "<") {
			b.variablesFile = strings.TrimSpace(strings.TrimPrefix(trimmed, "<"))
			b.variablesLines = nil
			return true
		}
		if strings.HasPrefix(trimmed, "@") && strings.Contains(trimmed, "<") {
			parts := strings.SplitN(trimmed, "<", 2)
			if len(parts) == 2 {
				b.variablesFile = strings.TrimSpace(parts[1])
				b.variablesLines = nil
				return true
			}
		}
		b.variablesLines = append(b.variablesLines, line)
		return true
	}

	if strings.HasPrefix(trimmed, "<") {
		b.queryFile = strings.TrimSpace(strings.TrimPrefix(trimmed, "<"))
		b.queryLines = nil
		return true
	}

	if strings.HasPrefix(trimmed, "@") && strings.Contains(trimmed, "<") {
		parts := strings.SplitN(trimmed, "<", 2)
		if len(parts) == 2 {
			b.queryFile = strings.TrimSpace(parts[1])
			b.queryLines = nil
			return true
		}
	}
	b.queryLines = append(b.queryLines, line)
	return true
}

func (b *Builder) Finalize(existingMime string) (*restfile.GraphQLBody, string, bool) {
	if !b.enabled {
		return nil, existingMime, false
	}

	gql := &restfile.GraphQLBody{
		Query:         strings.TrimSpace(strings.Join(b.queryLines, "\n")),
		OperationName: strings.TrimSpace(b.operation),
		Variables:     strings.TrimSpace(strings.Join(b.variablesLines, "\n")),
	}

	if b.queryFile != "" {
		gql.QueryFile = b.queryFile
	}
	if b.variablesFile != "" {
		gql.VariablesFile = b.variablesFile
	}

	mime := existingMime
	if mime == "" {
		mime = "application/json"
	}
	return gql, mime, true
}
