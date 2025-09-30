package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/quick"
)

func prettifyBody(body []byte, contentType string) string {
	ct := strings.ToLower(contentType)
	source := string(body)
	lexer := ""

	switch {
	case strings.Contains(ct, "json"):
		if formatted, ok := renderJSONAsJS(body); ok {
			source = formatted
			lexer = "javascript"
		} else {
			var buf bytes.Buffer
			if err := json.Indent(&buf, body, "", "  "); err == nil {
				source = buf.String()
			}
			lexer = "json"
		}
	case strings.Contains(ct, "xml"):
		lexer = "xml"
	case strings.Contains(ct, "html"):
		lexer = "html"
	case strings.Contains(ct, "yaml"):
		lexer = "yaml"
	case strings.Contains(ct, "javascript") || strings.Contains(ct, "ecmascript"):
		lexer = "javascript"
	}

	if lexer == "" {
		return source
	}

	if highlighted, ok := highlight(source, lexer); ok {
		return highlighted
	}

	return source
}

func renderJSONAsJS(body []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var value interface{}
	if err := dec.Decode(&value); err != nil {
		return "", false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return "", false
	}
	buf := strings.Builder{}
	writeJSONValue(&buf, value, 0)
	return buf.String(), true
}

func writeJSONValue(buf *strings.Builder, value interface{}, indent int) {
	switch v := value.(type) {
	case map[string]interface{}:
		writeJSONObject(buf, v, indent)
	case []interface{}:
		writeJSONArray(buf, v, indent)
	case json.Number:
		buf.WriteString(v.String())
	case string:
		buf.WriteString(strconv.Quote(v))
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			buf.WriteString(strconv.Quote(fmt.Sprintf("%v", v)))
		} else {
			buf.Write(encoded)
		}
	}
}

func writeJSONObject(buf *strings.Builder, obj map[string]interface{}, indent int) {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}

	sort.Strings(keys)
	if len(keys) == 0 {
		buf.WriteString("{}")
		return
	}

	buf.WriteString("{\n")
	for i, key := range keys {
		buf.WriteString(strings.Repeat("  ", indent+1))
		buf.WriteString(formatJSProperty(key))
		buf.WriteString(": ")
		writeJSONValue(buf, obj[key], indent+1)
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(strings.Repeat("  ", indent))
	buf.WriteString("}")
}

func writeJSONArray(buf *strings.Builder, arr []interface{}, indent int) {
	if len(arr) == 0 {
		buf.WriteString("[]")
		return
	}

	buf.WriteString("[\n")
	for i, item := range arr {
		buf.WriteString(strings.Repeat("  ", indent+1))
		writeJSONValue(buf, item, indent+1)
		if i < len(arr)-1 {
			buf.WriteString(",")
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(strings.Repeat("  ", indent))
	buf.WriteString("]")
}

func formatJSProperty(name string) string {
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
			if !(unicode.IsLetter(r) || r == '_' || r == '$') {
				return false
			}
			continue
		}

		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$') {
			return false
		}
	}
	return true
}

func highlight(content, lexer string) (string, bool) {
	var buf bytes.Buffer
	if err := quick.Highlight(&buf, content, lexer, "terminal16m", "monokai"); err != nil {
		return "", false
	}
	return buf.String(), true
}
