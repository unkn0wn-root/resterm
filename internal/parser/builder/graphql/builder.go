package graphql

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/parser/bodyref"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	str "github.com/unkn0wn-root/resterm/internal/util"
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

		rest = str.Trim(rest)
		if rest != "" {
			if file, ok := bodyref.Parse(rest, bodyref.Options{
				Location: bodyref.Line,
			}); ok {
				b.variablesFile = file
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

		rest = str.Trim(rest)
		if rest != "" {
			if file, ok := bodyref.Parse(rest, bodyref.Options{
				Location: bodyref.Line,
			}); ok {
				b.queryFile = file
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

func (b *Builder) HandleBodyLine(line string) bool {
	if !b.enabled {
		return false
	}
	if b.collectVariables {
		if file, ok := bodyref.Parse(line, bodyref.Options{
			Location: bodyref.Line,
		}); ok {
			b.variablesFile = file
			b.variablesLines = nil
			return true
		}
		if file, ok := bodyref.Parse(line, bodyref.Options{
			Location: bodyref.Inline,
		}); ok {
			b.variablesFile = file
			b.variablesLines = nil
			return true
		}
		b.variablesLines = append(b.variablesLines, line)
		return true
	}

	if file, ok := bodyref.Parse(line, bodyref.Options{
		Location: bodyref.Line,
	}); ok {
		b.queryFile = file
		b.queryLines = nil
		return true
	}

	if file, ok := bodyref.Parse(line, bodyref.Options{
		Location: bodyref.Inline,
	}); ok {
		b.queryFile = file
		b.queryLines = nil
		return true
	}
	b.queryLines = append(b.queryLines, line)
	return true
}

func (b *Builder) Finalize(existingMime string) (*restfile.GraphQLBody, string, bool) {
	if !b.enabled {
		return nil, existingMime, false
	}

	gql := &restfile.GraphQLBody{
		Query:         str.Trim(strings.Join(b.queryLines, "\n")),
		OperationName: str.Trim(b.operation),
		Variables:     str.Trim(strings.Join(b.variablesLines, "\n")),
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
