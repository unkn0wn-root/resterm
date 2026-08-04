package bodyfmt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	js "github.com/unkn0wn-root/resterm/internal/parser/javascript"
)

// RenderJSONAsJS formats a JSON body as a JavaScript object literal: unquoted
// keys where they are valid identifiers, and numbers kept verbatim. The JS
// parser handles it when the body is already literal syntax; otherwise we
// decode it and print it ourselves.
func RenderJSONAsJS(ctx context.Context, body []byte) (string, bool) {
	if done(ctx) {
		return "", false
	}
	if formatted, err := js.FormatValue(string(body)); err == nil {
		return formatted, true
	}
	if done(ctx) {
		return "", false
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	var value any
	if err := dec.Decode(&value); err != nil {
		return "", false
	}
	// Trailing content means this was never a single JSON value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", false
	}

	var p jsPrinter
	p.value(value)
	return p.buf.String(), true
}

type jsPrinter struct {
	buf   strings.Builder
	depth int
}

func (p *jsPrinter) value(value any) {
	switch v := value.(type) {
	case map[string]any:
		p.object(v)
	case []any:
		p.array(v)
	case json.Number:
		p.buf.WriteString(v.String())
	case string:
		if formatted, ok := js.FormatInlineValue(v, p.depth); ok {
			p.buf.WriteString(formatted)
			return
		}
		p.buf.WriteString(strconv.Quote(v))
	case bool:
		p.buf.WriteString(strconv.FormatBool(v))
	case nil:
		p.buf.WriteString("null")
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			p.buf.WriteString(strconv.Quote(fmt.Sprint(v)))
			return
		}
		p.buf.Write(encoded)
	}
}

func (p *jsPrinter) object(obj map[string]any) {
	if len(obj) == 0 {
		p.buf.WriteString("{}")
		return
	}

	keys := slices.Sorted(maps.Keys(obj))
	p.open('{')
	for i, key := range keys {
		p.pad()
		p.buf.WriteString(jsProperty(key))
		p.buf.WriteString(": ")
		p.value(obj[key])
		p.end(i, len(keys))
	}
	p.close('}')
}

func (p *jsPrinter) array(arr []any) {
	if len(arr) == 0 {
		p.buf.WriteString("[]")
		return
	}

	p.open('[')
	for i, item := range arr {
		p.pad()
		p.value(item)
		p.end(i, len(arr))
	}
	p.close(']')
}

func (p *jsPrinter) open(brace byte) {
	p.buf.WriteByte(brace)
	p.buf.WriteByte('\n')
	p.depth++
}

func (p *jsPrinter) close(brace byte) {
	p.depth--
	p.pad()
	p.buf.WriteByte(brace)
}

func (p *jsPrinter) pad() {
	p.buf.WriteString(strings.Repeat("  ", p.depth))
}

func (p *jsPrinter) end(i, n int) {
	if i < n-1 {
		p.buf.WriteByte(',')
	}
	p.buf.WriteByte('\n')
}

func jsProperty(name string) string {
	if isJSIdentifier(name) {
		return name
	}
	return strconv.Quote(name)
}

func isJSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' && r != '$' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$' {
			return false
		}
	}
	return true
}
