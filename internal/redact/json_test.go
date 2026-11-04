package redact

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func goJSON(v any, escapeHTML bool) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(escapeHTML)
	_ = enc.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

func asciiJSON(v any, format string) []byte {
	var b strings.Builder
	for _, r := range string(goJSON(v, false)) {
		switch {
		case r < utf8.RuneSelf:
			b.WriteRune(r)
		case r > 0xffff:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, format+format, hi, lo)
		default:
			fmt.Fprintf(&b, format, r)
		}
	}
	return []byte(b.String())
}

func dotnetJSON(v any) []byte {
	src := string(goJSON(v, false))
	var b strings.Builder
	for i := 0; i < len(src); {
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case r == '\\' && src[i+1] == '"':
			b.WriteString("\\u0022")
			size = 2
		case r == '\\':
			b.WriteString(src[i : i+2])
			size = 2
		case r > 0xffff:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, "\\u%04X\\u%04X", hi, lo)
		case strings.ContainsRune("+'`<>&", r) || r > 0x7e:
			fmt.Fprintf(&b, "\\u%04X", r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return []byte(b.String())
}

var encoders = []struct {
	name string
	enc  func(any) []byte
}{
	{"go", func(v any) []byte { return goJSON(v, true) }},
	{"go without html escapes", func(v any) []byte { return goJSON(v, false) }},
	{"python", func(v any) []byte { return asciiJSON(v, "\\u%04x") }},
	{"php", func(v any) []byte { return bytes.ReplaceAll(asciiJSON(v, "\\u%04x"), []byte("/"), []byte(`\/`)) }},
	{"dotnet", dotnetJSON},
}

func jsonLeaks(t *testing.T, b []byte, sec string) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("masked body is not JSON: %v\n%s", err, b)
		}
		switch v := tok.(type) {
		case string:
			var inner any
			if json.Unmarshal([]byte(v), &inner) == nil {
				nested, _ := json.Marshal(inner)
				out = append(out, jsonLeaks(t, nested, sec)...)
			}
			if strings.Contains(v, sec) {
				out = append(out, v)
			}
		case json.Number:
			if strings.Contains(string(v), sec) {
				out = append(out, string(v))
			}
		}
	}
}

func TestJSONEncoders(t *testing.T) {
	const note = "keepé / <ok> & 'q'"
	for _, e := range encoders {
		for _, sec := range hard {
			t.Run(e.name+"/"+strconv.Quote(sec), func(t *testing.T) {
				body := e.enc(map[string]any{"id": 42, "note": note, "token": sec, sec: "as key"})
				got := New(sec).JSON(body)
				if leaks := jsonLeaks(t, got, sec); len(leaks) > 0 {
					t.Fatalf("%s leaks %q in %s", body, leaks, got)
				}
				keep := e.enc(map[string]any{"note": note})
				if !bytes.Contains(got, keep[1:len(keep)-1]) || !bytes.Contains(got, []byte(`"id":42`)) {
					t.Fatalf("other members changed:\n%s\nwant %s kept", got, keep)
				}
			})
		}
	}
}

func TestJSONNested(t *testing.T) {
	const sec = "a/b9"
	inner := string(bytes.ReplaceAll(asciiJSON(map[string]any{"t": sec}, "\\u%04x"), []byte("/"), []byte(`\/`)))
	outer := goJSON(map[string]any{"events": []any{map[string]any{"text": inner}}, "reason": "denied " + inner}, true)
	got := New(sec).JSON(outer)
	if leaks := jsonLeaks(t, got, sec); len(leaks) > 0 || bytes.Contains(got, []byte(`a\\\/b9`)) {
		t.Fatalf("JSON(%s) = %s leaks %q", outer, got, leaks)
	}
}

func TestJSONNumberKeyLiteral(t *testing.T) {
	got := New("123456", "true").JSON([]byte(`{"pin":123456,"ok":true,"123456":null,"n":-1234567e3}`))
	if want := `{"pin":"•••","ok":"•••","•••":null,"n":"-•••7e3"}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestJSONLeak14(t *testing.T) {
	got := New("nabc").JSON([]byte(`{"a":"x\nabc","b":"nabc"}`))
	if want := `{"a":"x\nabc","b":"•••"}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestJSONFallback(t *testing.T) {
	tests := map[string]string{
		"invalid":  `{"token":"a\/b9", oops`,
		"trailing": `{"token":"x a\/b9\`,
		"nonutf8":  "\xff\xfe{\"token\":\"a\\/b9\"}",
		"ndjson":   "{\"token\":\"a\\/b9\"}\n{\"token\":\"a\\u002fb9\"}\n",
		"text":     "tok=a%2fb9\n",
	}
	for name, body := range tests {
		got := string(New("a/b9").JSON([]byte(body)))
		if strings.Contains(got, "b9") || !strings.Contains(got, Mask) {
			t.Errorf("%s: JSON(%q) = %q", name, body, got)
		}
	}
}

func TestJSONInvalidUTF8Secret(t *testing.T) {
	const sec = "ab\xffcd"
	if got := New(sec).JSON([]byte("{\"k\":\"ab\xffcd\\n\"}")); bytes.Contains(got, []byte(sec)) {
		t.Fatalf("got %q", got)
	}
}

func TestJSONCrossToken(t *testing.T) {
	const body = `["a","/b9"]`
	if got := string(New(`a","/b9`).JSON([]byte(body))); got != body {
		t.Fatalf("got %s, want %s", got, body)
	}
}

func TestJSONKeepsPlain(t *testing.T) {
	body := []byte("{\n  \"token\": \"other\",\n  \"n\": [1, 2.5e3, true, null],\n  \"esc\": \"a\\/b \\u00e9\"\n}")
	for name, s := range map[string]Set{"zero Set": {}, "empty secret": New(""), "no secret": New("secret")} {
		if got := s.JSON(body); &got[0] != &body[0] || len(got) != len(body) {
			t.Errorf("%s: JSON returned %q, want the body itself", name, got)
		}
	}
}

func TestBase64Values(t *testing.T) {
	frame := base64.StdEncoding.EncodeToString([]byte("bin-send-binsec77"))
	body := []byte(`{"events":[{"type":"binary","base64":"` + frame + `"},{"type":"text","text":"hello"}]}`)
	want := `{"events":[{"type":"binary","base64":"•••"},{"type":"text","text":"hello"}]}`
	if got := New("binsec77").JSON(body); string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func FuzzJSON(f *testing.F) {
	for _, sec := range hard {
		for _, e := range encoders {
			f.Add(sec, e.enc(map[string]any{"token": sec, "id": 42, sec: []any{sec, 1.5, true, nil}}))
		}
	}
	f.Fuzz(func(t *testing.T, sec string, doc []byte) {
		if sec == "" || sharesMaskByte(sec) || !utf8.ValidString(sec) && strings.Contains(string(utf8.RuneError), sec) {
			t.Skip()
		}
		got := New(sec).JSON(doc)
		if !utf8.Valid(doc) || !json.Valid(doc) {
			if bytes.Contains(got, []byte(sec)) {
				t.Fatalf("JSON(%q) = %q keeps the secret %q", doc, got, sec)
			}
			return
		}
		if !json.Valid(got) {
			t.Fatalf("JSON(%s) = %s is not JSON", doc, got)
		}
		dec := json.NewDecoder(bytes.NewReader(got))
		dec.UseNumber()
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				return
			}
			if err != nil {
				t.Fatalf("JSON(%s) = %s: %v", doc, got, err)
			}
			text := fmt.Sprint(tok)
			switch tok.(type) {
			case json.Delim:
				continue
			case nil:
				text = "null"
			}
			if strings.Contains(text, sec) {
				t.Fatalf("JSON(%s) = %s keeps the secret %q in %q", doc, got, sec, text)
			}
		}
	})
}
