package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/unkn0wn-root/resterm/internal/rts"
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

// function names have their own colour, so a theme that recolours option keys
// leaves them alone
func TestRTSRuneStylerFunctionIgnoresSettingKey(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	p.SettingKey = lipgloss.Color("#123456")
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	src := "fn add(a, b) {"
	styles := rs.StylesForLine([]rune(src), 0)
	idx := strings.Index(src, "add")
	if got := styles[idx].GetForeground(); got != p.RTSFunction {
		t.Fatalf("fn name colour = %q, want %q", got, p.RTSFunction)
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

// reserving default removed the ambiguity that used to keep it unstyled: every
// spelling on any line is now the keyword, so one line is enough to classify it
func TestRTSRuneStylerHighlightsDefault(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	p.RTSKeywordControl = lipgloss.Color("#112233")
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	control, ok := rs.keywordStyleForClass(rts.KeywordControl)
	if !ok {
		t.Fatalf("expected control keyword style")
	}

	lines := []string{
		"  default:",
		"default:",
		"  default: 1",
	}
	for i, src := range lines {
		styles := rs.StylesForLine([]rune(src), i)
		idx := strings.Index(src, "default")
		got := styles[idx].GetForeground()
		if want := control.GetForeground(); got != want {
			t.Fatalf("%q color: got %v, want %v", src, got, want)
		}
	}

	// a quoted key is data, so it keeps the string colour
	styles := rs.StylesForLine([]rune(`{"default": 1}`), 0)
	idx := strings.Index(`{"default": 1}`, "default")
	if got := styles[idx].GetForeground(); got == control.GetForeground() {
		t.Fatalf(`"default" in a quoted key should not use the control keyword color`)
	}
}

func TestRTSRuneStylerHighlightsSwitchKeywords(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	p.RTSKeywordControl = lipgloss.Color("#112233")
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	control, ok := rs.keywordStyleForClass(rts.KeywordControl)
	if !ok {
		t.Fatalf("expected control keyword style")
	}

	for i, src := range []string{"switch code {", "case 200, 201:"} {
		styles := rs.StylesForLine([]rune(src), i)
		got := styles[0].GetForeground()
		if want := control.GetForeground(); got != want {
			t.Fatalf("%q color: got %v, want %v", src, got, want)
		}
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

func TestRTSRuneStylerHighlightsMembershipOperator(t *testing.T) {
	p := theme.DefaultTheme().EditorMetadata
	p.RTSKeywordLogical = lipgloss.Color("#112233")
	st := newRTSRuneStyler(p)
	rs, ok := st.(*rtsRuneStyler)
	if !ok {
		t.Fatalf("expected rts rune styler")
	}

	logical, ok := rs.keywordStyleForClass(rts.KeywordLogical)
	if !ok {
		t.Fatalf("expected logical keyword style")
	}

	tests := []struct {
		src      string
		at       string
		operator bool
	}{
		{src: "let ok = code in [200, 201]", at: "in [", operator: true},
		{src: "let ok = code not in [400]", at: "in [", operator: true},
		{src: `let ok = "json" in ct`, at: "in ct", operator: true},
		{src: "let ok = pick(1) in [1]", at: "in [", operator: true},
		{src: "let ok = true in [true]", at: "in [", operator: true},
		{src: "let ok = 1 in [1]", at: "in [", operator: true},
		{src: "let in = 1", at: "in =", operator: false},
		{src: "let x = in.field", at: "in.", operator: false},
		{src: "fn pick(in) {", at: "in)", operator: false},
		{src: "let d = {in: 1}", at: "in:", operator: false},
		{src: "let y = not in", at: "in", operator: false},
		{src: "in = pick(in)", at: "in =", operator: false},
	}
	for i, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			idx := strings.Index(tt.src, tt.at)
			if idx < 0 {
				t.Fatalf("anchor %q missing from %q", tt.at, tt.src)
			}

			styles := rs.StylesForLine([]rune(tt.src), i)
			got := styles[idx].GetForeground()
			want := logical.GetForeground()
			if tt.operator && got != want {
				t.Fatalf("in color = %v, want logical %v", got, want)
			}
			if !tt.operator && got == want {
				t.Fatalf("in used the logical keyword color as a name")
			}
		})
	}
}
