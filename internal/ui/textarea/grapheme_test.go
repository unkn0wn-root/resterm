package textarea

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/rivo/uniseg"
)

func TestViewKeepsGraphemesTogether(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	styles := []struct {
		name  string
		style lipgloss.Style
	}{
		{"plain", lipgloss.NewStyle()},
		{"syntax", lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00"))},
		{"underline", lipgloss.NewStyle().Underline(true)},
		{"strikethrough", lipgloss.NewStyle().Strikethrough(true)},
	}
	for _, cluster := range []string{"⚙️", "👩‍💻", "🇳🇴", "é", "👍🏽"} {
		for _, st := range styles {
			t.Run(cluster+"/"+st.name, func(t *testing.T) {
				m := newTextArea()
				m.Prompt = ""
				m.ShowLineNumbers = false
				m.SetHeight(1)
				m.SetWidth(20)
				m.SetValue("a" + cluster + "z")
				if st.name != "plain" {
					m.SetRuneStyler(fixedLineRuneStyler{0: st.style})
				}
				m.Cursor.SetMode(cursor.CursorStatic)
				for col := range len(m.value[0]) + 1 {
					m.SetCursor(col)
					for _, blink := range []bool{false, true} {
						m.Cursor.Blink = blink
						view := m.View()
						if !strings.Contains(view, cluster) {
							t.Fatalf("cursor %d, blink %t: escape sequences split %q: %q", col, blink, cluster, view)
						}
						if got := strings.TrimSpace(ansi.Strip(view)); got != m.Value() {
							t.Fatalf("cursor %d: visible text %q, want %q", col, got, m.Value())
						}
						if styled, visible := ansi.StringWidth(
							view,
						), ansi.StringWidth(
							ansi.Strip(view),
						); styled != visible ||
							visible != m.Width() {
							t.Fatalf(
								"cursor %d: styled width %d, visible width %d, want %d",
								col,
								styled,
								visible,
								m.Width(),
							)
						}
					}
				}
			})
		}
	}
}

func TestRenderingWithoutANSIStaysPlain(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	styles := []lipgloss.Style{
		lipgloss.NewStyle(),
		lipgloss.NewStyle().Underline(true),
		lipgloss.NewStyle().Strikethrough(true),
		lipgloss.NewStyle().Underline(true).Foreground(lipgloss.Color("#ffaa00")),
	}
	for _, text := range []string{"a", "é", "⚙️", "👩‍💻", "🇳🇴", "👍🏽"} {
		for _, style := range styles {
			if got := renderGrapheme(style, text); got != text {
				t.Errorf("renderGrapheme(%q) = %q, want it unchanged", text, got)
			}
		}
	}

	for _, cluster := range []string{"⚙️", "👩‍💻", "é"} {
		m := newTextArea()
		m.Prompt = ""
		m.ShowLineNumbers = false
		m.SetHeight(1)
		m.SetWidth(20)
		m.SetValue("a" + cluster + "z")
		m.SetRuneStyler(fixedLineRuneStyler{0: lipgloss.NewStyle().Underline(true)})
		m.Cursor.SetMode(cursor.CursorStatic)
		for col := range len(m.value[0]) + 1 {
			m.SetCursor(col)
			for _, blink := range []bool{false, true} {
				m.Cursor.Blink = blink
				if view := m.View(); strings.Contains(view, "\x1b") {
					t.Fatalf("cursor %d, blink %t: view carries escapes with ANSI off: %q", col, blink, view)
				}
			}
		}
	}
}

func TestVisibleSegmentKeepsWholeGraphemes(t *testing.T) {
	for _, cluster := range []string{"⚙️", "👩‍💻", "🇳🇴", "👍🏽"} {
		line := []rune("a" + cluster + "z")
		for _, tc := range []struct {
			offset, width int
			want          string
		}{
			{0, 2, "a"},
			{0, 3, "a" + cluster},
			{1, 1, ""},
			{1, 2, cluster},
			{2, 2, cluster},
			{3, 1, "z"},
		} {
			t.Run(fmt.Sprintf("%s/%d/%d", cluster, tc.offset, tc.width), func(t *testing.T) {
				_, got, width := visibleSegment(line, tc.offset, tc.width)
				if string(got) != tc.want || width != ansi.StringWidth(tc.want) {
					t.Fatalf(
						"visible segment %q (%d cells), want %q (%d cells)",
						string(got),
						width,
						tc.want,
						ansi.StringWidth(tc.want),
					)
				}
			})
		}
	}
}

func TestGraphemeSelectionAndSearchIncludeCombiningRunes(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })
	for _, selection := range []bool{false, true} {
		m := newTextArea()
		m.Prompt = ""
		m.ShowLineNumbers = false
		m.SetHeight(1)
		m.SetWidth(20)
		m.SetValue("a⚙️z")
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#ffaa00"))
		marked := lipgloss.NewStyle().Background(lipgloss.Color("#334455"))
		m.SetRuneStyler(fixedLineRuneStyler{0: style})
		if selection {
			m.SetSelectionStyle(marked)
			m.SetSelectionRange(2, 3)
		} else {
			m.SetHighlightStyle(marked)
			m.SetHighlightRanges([]HighlightRange{{Start: 2, End: 3}})
		}
		for col := range len(m.value[0]) + 1 {
			m.SetCursor(col)
			m.Cursor.Blink = true
			view := m.View()
			want := marked.Inherit(style).Render("⚙️")
			if !strings.Contains(view, want) {
				t.Fatalf("selection %t, cursor %d: missing marked grapheme %q in %q", selection, col, want, view)
			}
		}
	}
}

// Mix ASCII and Unicode classes to test transitions between the fast path
// and uniseg.
var clusterAtoms = []string{
	"a", "Z", "0", " ", "{", "}", ":", "\"", "~", "!", "\x7f", "\x01", "\r", "\v",
	"⚙", "️", "︎", "‍", "́", "̀", "​",
	"👩", "💻", "👍", "🏽", "🇳", "🇴", "🇺", "🏳", "🌈", "❤",
	"你", "好", "€", "→", "█", "ก", "ำ", "ᄀ", "ᅡ", "ᆨ", "؀", "؅",
	"अ", "ि", "﷽", "⃣", "#", "*",
}

func randomClusterLine(r *rand.Rand, n int) string {
	var b strings.Builder
	for range n {
		b.WriteString(clusterAtoms[r.Intn(len(clusterAtoms))])
	}
	return b.String()
}

func TestClustersMatchSegmenter(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for range 20000 {
		text := randomClusterLine(r, r.Intn(12))
		line := []rune(text)

		var want []Cluster
		g := uniseg.NewGraphemes(text)
		for start := 0; g.Next(); {
			end := start + utf8.RuneCountInString(g.Str())
			want = append(want, Cluster{Start: start, End: end, Width: g.Width(), Text: g.Str()})
			start = end
		}

		var got []Cluster
		for c := range Clusters(line) {
			got = append(got, c)
		}
		if len(got) != len(want) {
			t.Fatalf("%q: got %d clusters, want %d", text, len(got), len(want))
		}
		for i, c := range got {
			if c != want[i] {
				t.Fatalf("%q cluster %d: got %+v, want %+v", text, i, c, want[i])
			}
		}
	}
}

func TestViewWidthHoldsAcrossUnicode(t *testing.T) {
	profile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(profile) })

	r := rand.New(rand.NewSource(7))
	for range 300 {
		content := randomClusterLine(r, 1+r.Intn(20)) + "\n" + randomClusterLine(r, 1+r.Intn(20))
		m := newTextArea()
		m.Prompt = ""
		m.ShowLineNumbers = false
		m.SetHeight(3)
		m.SetWidth(1 + r.Intn(40))
		m.SetValue(content)
		if r.Intn(2) == 0 {
			m.SetRuneStyler(fixedLineRuneStyler{0: lipgloss.NewStyle().Underline(true)})
		}
		if r.Intn(3) == 0 {
			m.SetSelectionStyle(lipgloss.NewStyle().Background(lipgloss.Color("#334455")))
			a, b := r.Intn(len(m.Value())+1), r.Intn(len(m.Value())+1)
			m.SetSelectionRange(min(a, b), max(a, b))
		}
		if r.Intn(3) == 0 {
			a := r.Intn(len([]rune(content)) + 1)
			m.SetHighlightStyle(lipgloss.NewStyle().Background(lipgloss.Color("#553311")))
			m.SetHighlightRanges([]HighlightRange{{Start: a, End: a + 1 + r.Intn(5)}})
		}
		for row := range 2 {
			m.row = row
			for col := 0; col <= len(m.value[row]); col++ {
				m.SetCursor(col)
				m.Cursor.Blink = col%2 == 0
				for i, line := range strings.Split(m.View(), "\n") {
					if line == "" {
						continue
					}
					styled, visible := ansi.StringWidth(line), ansi.StringWidth(ansi.Strip(line))
					if styled != visible || visible != m.Width() {
						t.Fatalf("w=%d row=%d col=%d line=%d: styled %d, visible %d, want %d\n%q",
							m.Width(), row, col, i, styled, visible, m.Width(), line)
					}
				}
			}
		}
	}
}

var longLine = []rune(strings.Repeat(`{"key":"value","n":12345},`, 400))
var normalDoc = strings.Repeat("GET https://api.example.com/v1/resources?limit=50&offset=0 HTTP/1.1\n", 200)

func BenchmarkVisualWidth(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = visualWidth(longLine)
	}
}
func BenchmarkVisualWidthUntil(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = visualWidthUntil(longLine, 40)
	}
}
func BenchmarkVisibleSegment(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _, _ = visibleSegment(longLine, 5000, 120)
	}
}
func BenchmarkViewLongLine(b *testing.B) {
	m := newTextArea()
	m.SetWidth(120)
	m.SetHeight(40)
	m.SetValue(strings.Repeat(string(longLine)+"\n", 40))
	m.SetCursor(5000)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
func BenchmarkViewNormalDoc(b *testing.B) {
	m := newTextArea()
	m.SetWidth(120)
	m.SetHeight(40)
	m.SetValue(normalDoc)
	m.SetCursor(10)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.View()
	}
}
