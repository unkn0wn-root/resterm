package bodyfmt

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/unkn0wn-root/resterm/internal/binaryview"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestBuildQuotesUnprintableRunes(t *testing.T) {
	// UTF-8 "i acute" read as Latin-1 becomes U+00C3 and a soft hyphen.
	const value = "tecnolog\u00c3\u00ada \u202e \u2060 \u200b"
	tests := []struct {
		name        string
		contentType string
		body        string
	}{
		{"json", "application/json", `{"d":"` + value + `"}`},
		{"malformed json", "application/json", `{"d":"` + value + `"`},
		{"xml", "application/xml", "<d>" + value + "</d>"},
		{"text", "text/plain", value},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			views := build(BuildInput{
				Body:        []byte(tt.body),
				ContentType: tt.contentType,
				Form:        Display,
			})
			for name, view := range map[string]string{
				"pretty":   views.Pretty,
				"raw":      views.Raw,
				"raw text": views.RawText,
			} {
				if strings.ContainsAny(view, "\u00ad\u202e\u2060\u200b") {
					t.Errorf("%s kept an unprintable rune: %q", name, view)
				}
				if !strings.Contains(view, `\u00ad`) {
					t.Errorf("%s dropped the soft hyphen instead of quoting it: %q", name, view)
				}
				if !strings.Contains(view, "tecnolog") {
					t.Errorf("%s changed the surrounding text: %q", name, view)
				}
			}
		})
	}
}

func TestHeaderFieldsQuoteUnprintableRunes(t *testing.T) {
	fields := HeaderFields(http.Header{"X-Note": {"caf\u00e9\u00ad \u200b done"}})
	want := HeaderField{Name: "X-Note", Value: "caf\u00e9" + `\u00ad \u200b done`}
	if len(fields) != 1 || fields[0] != want {
		t.Fatalf("HeaderFields()=%v, want [%v]", fields, want)
	}
}

func TestDisplayFormattingPreservesOriginalBytes(t *testing.T) {
	body := []byte("soft\u00ad\tvalue\r\n")
	original := bytes.Clone(body)
	views := build(BuildInput{Body: body, ContentType: "text/plain", Form: Display})
	if views.RawText != "soft\\u00ad    value" {
		t.Fatalf("unexpected display text: %q", views.RawText)
	}
	decoded, err := base64.StdEncoding.DecodeString(views.RawBase64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, original) || !bytes.Equal(body, original) {
		t.Fatal("display formatting changed the original bytes or binary dump")
	}
}

func TestPrettifyQuotesInputEscapesBeforeHighlighting(t *testing.T) {
	body := []byte("// soft\u00ad \x1b[2J\nconst n = 42;")
	view := Prettify(t.Context(), body, "text/javascript", PrettyOptions{Color: termcolor.TrueColor(), Form: Display})
	if strings.Contains(view, "\x1b[2J") || strings.ContainsRune(view, '\u00ad') {
		t.Fatalf("input controls reached the terminal: %q", view)
	}
	plain := ansi.Strip(view)
	if view == plain || !strings.Contains(plain, `\u001b[2J`) || !strings.Contains(plain, `\u00ad`) {
		t.Fatalf("expected syntax colors and visible escaped input: %q", view)
	}
}

func TestBinarySummaryQuotesMetadata(t *testing.T) {
	p := Payload{Meta: binaryview.Meta{
		MIME: "application/soft\u00ad", DecodeErr: "bad\x1b[2J",
	}, Size: 1}
	for _, view := range []string{p.BinarySummaryText(), p.RawSummaryText()} {
		if strings.ContainsRune(view, '\u00ad') || strings.ContainsRune(view, '\x1b') {
			t.Fatalf("unescaped metadata in binary summary: %q", view)
		}
	}
}

func TestBuildPreservesBytesForDataConsumers(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		want        string
	}{
		{"tsv", "text/tab-separated-values", "name\tvalue\nrow\tcell", "\t"},
		{"crlf", "text/plain", "line one\r\nline two", "\r\n"},
		{"soft hyphen", "text/plain", "soft\u00adhyphen", "\u00ad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			views := build(BuildInput{Body: []byte(tt.body), ContentType: tt.contentType})
			if !strings.Contains(views.Raw, tt.want) {
				t.Errorf("Raw = %q, want it to keep %q", views.Raw, tt.want)
			}
			if !strings.Contains(views.RawText, tt.want) {
				t.Errorf("RawText = %q, want it to keep %q", views.RawText, tt.want)
			}
		})
	}
}

func TestBuildKeepsXMLTextValue(t *testing.T) {
	const want = "soft\u00adhyphen"
	views := build(BuildInput{
		Body:        []byte("<r><v>" + want + "</v></r>"),
		ContentType: "application/xml",
	})
	var out struct {
		V string `xml:"v"`
	}
	if err := xml.Unmarshal([]byte(views.Raw), &out); err != nil {
		t.Fatalf("raw body no longer parses as XML: %v", err)
	}
	if out.V != want {
		t.Errorf("XML text = %q, want %q", out.V, want)
	}
}
