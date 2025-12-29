package ui

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/theme"
)

func TestRTSRuneStylerHighlightsFnDecl(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	src := "fn add(a, b) {"
	line := []rune(src)
	styles := rs.StylesForLine(line, 0)
	if styles == nil {
		t.Fatalf("expected styles for rts line")
	}
	idx := strings.Index(src, "add")
	if idx < 0 {
		t.Fatalf("expected fn name in line")
	}
	style, ok := rs.nameStyle(line, idx, true)
	if !ok {
		t.Fatalf("expected function style")
	}
	got := styles[idx].Render("a")
	want := style.Render("a")
	if got != want {
		t.Fatalf("fn name style mismatch:\nwant %q\n got %q", want, got)
	}
}

func TestRTSRuneStylerHighlightsFnCall(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	src := "total = sum(1, 2)"
	line := []rune(src)
	styles := rs.StylesForLine(line, 0)
	if styles == nil {
		t.Fatalf("expected styles for rts line")
	}
	idx := strings.Index(src, "sum")
	if idx < 0 {
		t.Fatalf("expected fn call in line")
	}
	style, ok := rs.nameStyle(line, idx, false)
	if !ok {
		t.Fatalf("expected call style")
	}
	got := styles[idx].Render("s")
	want := style.Render("s")
	if got != want {
		t.Fatalf("fn call style mismatch:\nwant %q\n got %q", want, got)
	}
}

func TestRTSRuneStylerHighlightsMethodCall(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	src := "request.setHeader(\"X-Test\", \"1\")"
	line := []rune(src)
	styles := rs.StylesForLine(line, 0)
	if styles == nil {
		t.Fatalf("expected styles for rts line")
	}
	idx := strings.Index(src, "setHeader")
	if idx < 0 {
		t.Fatalf("expected method call in line")
	}
	style, ok := rs.nameStyle(line, idx, false)
	if !ok {
		t.Fatalf("expected method style")
	}
	got := styles[idx].Render("s")
	want := style.Render("s")
	if got != want {
		t.Fatalf("method call style mismatch:\nwant %q\n got %q", want, got)
	}
}
