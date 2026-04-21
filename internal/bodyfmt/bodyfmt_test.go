package bodyfmt

import (
	"net/http"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
)

func TestBuildJSONPlainText(t *testing.T) {
	views := Build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
	})
	if strings.Contains(views.Pretty, "\x1b[") {
		t.Fatalf("expected plain pretty output, got %q", views.Pretty)
	}
	if !strings.Contains(views.Pretty, `a: "x"`) {
		t.Fatalf("expected formatted json body, got %q", views.Pretty)
	}
}

func TestBuildBinaryDefaultsToHex(t *testing.T) {
	views := Build(BuildInput{
		Body:        []byte{0x00, 0x01, 0x02, 0x03},
		ContentType: "application/octet-stream",
	})
	if views.Mode != RawHex {
		t.Fatalf("Build(...).Mode=%v, want %v", views.Mode, RawHex)
	}
	if !strings.Contains(views.Pretty, "Binary body") {
		t.Fatalf("expected binary summary, got %q", views.Pretty)
	}
	if views.RawHex == "" || views.Raw != views.RawHex {
		t.Fatalf("expected default raw hex view, got raw=%q hex=%q", views.Raw, views.RawHex)
	}
}

func TestBuildJSONColorUsesResolvedFormatter(t *testing.T) {
	views := Build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
		Color:       termcolor.Enabled(termenv.ANSI256),
	})
	if !strings.Contains(views.Pretty, "\x1b[") {
		t.Fatalf("expected colored pretty output, got %q", views.Pretty)
	}
	if got := ansi.Strip(views.Pretty); !strings.Contains(got, `a: "x"`) {
		t.Fatalf("expected colored output to preserve text, got %q", got)
	}
}

func TestBuildJSONColorRespectsConfiguredStyle(t *testing.T) {
	github := Build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
		Color:       termcolor.Enabled(termenv.ANSI256),
		Style:       "github",
	})
	monokai := Build(BuildInput{
		Body:        []byte(`{"b":1,"a":"x"}`),
		ContentType: "application/json",
		Color:       termcolor.Enabled(termenv.ANSI256),
		Style:       "monokai",
	})

	if github.Pretty == monokai.Pretty {
		t.Fatalf("expected different ANSI output for different styles")
	}
	if ansi.Strip(github.Pretty) != ansi.Strip(monokai.Pretty) {
		t.Fatalf("expected different styles to preserve the same body text")
	}
}

func TestFormatHeadersSortsNamesAndValues(t *testing.T) {
	headers := http.Header{
		"X-B": {"2", "1"},
		"X-A": {"z"},
	}
	got := FormatHeaders(headers)
	want := "X-A: z\nX-B: 1, 2"
	if got != want {
		t.Fatalf("FormatHeaders()=%q, want %q", got, want)
	}
}

func TestStripANSIUsesLegacyUIBehavior(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "csi",
			in:   "\x1b[31mred\x1b[0m",
			want: "red",
		},
		{
			name: "osc",
			in:   "\x1b]8;;https://example.com\x07label\x1b]8;;\x07",
			want: "label",
		},
		{
			name: "incomplete csi preserved",
			in:   "\x1b[31",
			want: "\x1b[31",
		},
		{
			name: "non csi escape preserved",
			in:   "\x1bc",
			want: "\x1bc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripANSI(tt.in); got != tt.want {
				t.Fatalf("StripANSI(%q)=%q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
