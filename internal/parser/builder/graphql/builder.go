package graphql

import (
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
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

func (b *Builder) HandleDirective(name directive.Name, rest string) (handled, reset bool, err error) {
	switch name {
	case directive.GraphQL:
		reset, err := b.toggle(str.Trim(rest))
		return true, reset, err
	case directive.GraphQLOperation:
		if b.enabled {
			b.operation = rest
		}
		return b.enabled, false, nil
	case directive.Variables:
		if !b.enabled {
			return false, false, nil
		}
		b.collectVariables = true
		b.variablesLines = nil
		b.variablesFile = ""

		rest = str.Trim(rest)
		if rest != "" {
			if file, ok := bodyref.Parse(rest, bodyref.Options{Location: bodyref.Line}); ok {
				b.variablesFile = file
			} else {
				b.variablesLines = append(b.variablesLines, rest)
			}
		}
		return true, false, nil
	case directive.Query:
		if !b.enabled {
			return false, false, nil
		}
		b.collectVariables = false
		b.queryLines = nil
		b.queryFile = ""

		rest = str.Trim(rest)
		if rest != "" {
			if file, ok := bodyref.Parse(rest, bodyref.Options{Location: bodyref.Line}); ok {
				b.queryFile = file
				return true, false, nil
			}
			b.queryLines = append(b.queryLines, rest)
		}
		return true, false, nil
	}
	return false, false, nil
}

func (b *Builder) toggle(rest string) (reset bool, err error) {
	switch {
	case rest == "":
		b.enabled = true
	case directive.IsOff(rest):
		b.disable()
		return true, nil
	default:
		on, ok := directive.ParseBool(rest)
		if !ok {
			return false, fmt.Errorf("invalid @graphql %q: expected true or false", rest)
		}
		b.enabled = on
	}
	return false, nil
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

func (b *Builder) HandleBodyLine(line string, forceInline bool) bool {
	if !b.enabled {
		return false
	}
	if b.collectVariables {
		if file, ok := bodyref.ParseBodyFile(line, forceInline); ok {
			b.variablesFile = file
			b.variablesLines = nil
			return true
		}
		b.variablesLines = append(b.variablesLines, line)
		return true
	}

	if file, ok := bodyref.ParseBodyFile(line, forceInline); ok {
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
