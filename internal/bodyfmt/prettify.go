package bodyfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"

	"github.com/alecthomas/chroma"
	"github.com/alecthomas/chroma/formatters"
	"github.com/alecthomas/chroma/lexers/h"
	"github.com/alecthomas/chroma/lexers/j"
	"github.com/alecthomas/chroma/lexers/x"
	"github.com/alecthomas/chroma/lexers/y"
	"github.com/alecthomas/chroma/styles"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/termtext"
)

const defaultSyntaxStyle = "monokai"

// TextForm selects between the bytes the response carried and a form that is
// safe to draw in a terminal. Escaping rewrites tabs and invisible characters,
// so it must not reach callers that treat the body as data, such as the CLI
// --body flag. The zero value keeps the original bytes.
type TextForm int

const (
	Original TextForm = iota
	Display
)

type PrettyOptions struct {
	Color termcolor.Config
	Style string
	Form  TextForm
}

// syntax is what we make of a Content-Type: it drives both reindentation and
// the chroma lexer, so the content type is only matched in one place.
type syntax int

const (
	syntaxPlain syntax = iota
	syntaxJSON
	syntaxXML
	syntaxHTML
	syntaxYAML
	syntaxJS
)

func detect(contentType string) syntax {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		return syntaxJSON
	case strings.Contains(ct, "xml"):
		return syntaxXML
	case strings.Contains(ct, "html"):
		return syntaxHTML
	case strings.Contains(ct, "yaml"):
		return syntaxYAML
	case strings.Contains(ct, "javascript"), strings.Contains(ct, "ecmascript"):
		return syntaxJS
	default:
		return syntaxPlain
	}
}

// Direct lexer imports avoid linking every language into the binary.
func (s syntax) lexer() chroma.Lexer {
	switch s {
	case syntaxJSON:
		return j.JSON
	case syntaxXML:
		return x.XML
	case syntaxHTML:
		return h.HTML
	case syntaxYAML:
		return y.YAML
	case syntaxJS:
		return j.Javascript
	default:
		return nil
	}
}

// Prettify re-indents and syntax highlights the body, preserving readable text
// if either step fails. Escape after reindentation so parsers see the original
// body, and before highlighting so generated ANSI escapes remain intact.
func Prettify(ctx context.Context, body []byte, contentType string, opt PrettyOptions) string {
	out, lang := reindent(ctx, body, contentType)
	if opt.Form == Display {
		out = termtext.Block(out)
	}

	lexer := lang.lexer()
	if !opt.Color.Enabled || lexer == nil || done(ctx) {
		return out
	}
	if highlighted, ok := highlight(out, lexer, opt.Color, opt.Style); ok {
		return highlighted
	}
	return out
}

// reindent returns the output syntax for highlighting, which may differ from
// the input content type.
func reindent(ctx context.Context, body []byte, contentType string) (string, syntax) {
	out := string(body)
	if done(ctx) {
		return out, syntaxPlain
	}

	lang := detect(contentType)
	switch lang {
	case syntaxJSON:
		// JSON renders as JS object literal syntax when it parses, which reads
		// better than quoted keys and matches the script editor.
		if formatted, ok := RenderJSONAsJS(ctx, body); ok {
			return formatted, syntaxJS
		}
		if done(ctx) {
			return out, lang
		}
		if indented, ok := indentJSON(body); ok {
			out = indented
		}
	case syntaxXML:
		if indented, ok := indentXML(body); ok {
			out = indented
		}
	}
	return out, lang
}

// FormatRaw re-indents the body without colouring it.
func FormatRaw(body []byte, contentType string, form TextForm) string {
	out, ok := indent(body, contentType)
	if !ok {
		out = string(body)
	}
	if form == Display {
		out = termtext.Block(out)
	}
	return TrimBody(out)
}

func indent(body []byte, contentType string) (string, bool) {
	switch detect(contentType) {
	case syntaxJSON:
		return indentJSON(body)
	case syntaxXML:
		return indentXML(body)
	default:
		return "", false
	}
}

func indentJSON(body []byte) (string, bool) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		return "", false
	}
	return buf.String(), true
}

func indentXML(body []byte) (string, bool) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false
		}
		if err := enc.EncodeToken(tok); err != nil {
			return "", false
		}
	}
	if err := enc.Flush(); err != nil {
		return "", false
	}
	return buf.String(), true
}

func highlight(content string, lexer chroma.Lexer, color termcolor.Config, style string) (string, bool) {
	formatter := color.Formatter()
	if formatter == "" {
		return "", false
	}
	if style = strings.TrimSpace(style); style == "" {
		style = defaultSyntaxStyle
	}

	it, err := chroma.Coalesce(lexer).Tokenise(nil, content)
	if err != nil {
		return "", false
	}
	var buf bytes.Buffer
	if err := formatters.Get(formatter).Format(&buf, styles.Get(style), it); err != nil {
		return "", false
	}
	return buf.String(), true
}
