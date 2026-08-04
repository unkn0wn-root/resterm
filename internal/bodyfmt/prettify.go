package bodyfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"strings"

	"github.com/alecthomas/chroma/quick"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

const defaultSyntaxStyle = "monokai"

type PrettyOptions struct {
	Color termcolor.Config
	Style string
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

func (s syntax) lexer() string {
	switch s {
	case syntaxJSON:
		return "json"
	case syntaxXML:
		return "xml"
	case syntaxHTML:
		return "html"
	case syntaxYAML:
		return "yaml"
	case syntaxJS:
		return "javascript"
	default:
		return ""
	}
}

// Prettify re-indents the body for its content type and syntax highlights it.
// Every step degrades to the text it started from, so a malformed body still
// comes back readable.
func Prettify(ctx context.Context, body []byte, contentType string, opt PrettyOptions) string {
	out := string(body)
	if done(ctx) {
		return out
	}

	lang := detect(contentType)
	switch lang {
	case syntaxJSON:
		// JSON renders as JS object literal syntax when it parses, which reads
		// better than quoted keys and matches the script editor.
		if formatted, ok := RenderJSONAsJS(ctx, body); ok {
			out = formatted
			lang = syntaxJS
			break
		}
		if done(ctx) {
			return out
		}
		if indented, ok := indentJSON(body); ok {
			out = indented
		}
	case syntaxXML:
		if indented, ok := indentXML(body); ok {
			out = indented
		}
	}

	lexer := lang.lexer()
	if !opt.Color.Enabled || lexer == "" || done(ctx) {
		return out
	}
	if highlighted, ok := highlight(out, lexer, opt.Color, opt.Style); ok {
		return highlighted
	}
	return out
}

// FormatRaw re-indents the body without colouring it.
func FormatRaw(body []byte, contentType string) string {
	if indented, ok := indent(body, contentType); ok {
		return TrimBody(indented)
	}
	return TrimBody(string(body))
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

func highlight(content, lexer string, color termcolor.Config, style string) (string, bool) {
	formatter := color.Formatter()
	if formatter == "" {
		return "", false
	}
	if style = strings.TrimSpace(style); style == "" {
		style = defaultSyntaxStyle
	}

	var buf bytes.Buffer
	if err := quick.Highlight(&buf, content, lexer, formatter, style); err != nil {
		return "", false
	}
	return buf.String(), true
}
