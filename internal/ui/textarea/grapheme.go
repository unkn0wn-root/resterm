package textarea

import (
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"

	"github.com/unkn0wn-root/resterm/internal/termtext"
)

// printableASCII avoids allocating a new string for each ASCII character.
var printableASCII = func() string {
	b := make([]byte, 0, 0x7f-' ')
	for c := byte(' '); c < 0x7f; c++ {
		b = append(b, c)
	}
	return string(b)
}()

// Cluster is a grapheme. Start and End are rune offsets; End is exclusive.
// Width is measured in terminal cells.
type Cluster struct {
	Start, End int
	Width      int
	Text       string
}

func printable(r rune) bool {
	return r >= ' ' && r < 0x7f
}

// asciiBreak reports whether a grapheme starts at i.
func asciiBreak(line []rune, i int) bool {
	return printable(line[i-1]) && printable(line[i])
}

// Clusters yields the graphemes in line.
//
// Adjacent printable ASCII runes always have a boundary between them,
// so they can bypass uniseg and safely delimit the chunks passed to it.
func Clusters(line []rune) iter.Seq[Cluster] {
	return func(yield func(Cluster) bool) {
		for i := 0; i < len(line); {
			if printable(line[i]) && (i+1 == len(line) || asciiBreak(line, i+1)) {
				at := line[i] - ' '
				if !yield(Cluster{Start: i, End: i + 1, Width: 1, Text: printableASCII[at : at+1]}) {
					return
				}
				i++
				continue
			}

			hi := i + 1
			for hi < len(line) && !asciiBreak(line, hi) {
				hi++
			}
			g := uniseg.NewGraphemes(string(line[i:hi]))
			for start := i; g.Next(); {
				text := g.Str()
				end := start + utf8.RuneCountInString(text)
				width := g.Width()
				// Invisible runes measure zero cells here but can take a cell in
				// the terminal. Draw an escape and keep Start and End on the runes.
				if esc, ok := termtext.EscapeCluster(text, width); ok {
					text, width = esc, uniseg.StringWidth(esc)
				}
				if !yield(Cluster{Start: start, End: end, Width: width, Text: text}) {
					return
				}
				start = end
			}
			i = hi
		}
	}
}

// clusterText builds the text Clusters would draw, for render paths that write
// a whole run at once.
func clusterText(line []rune) string {
	var b strings.Builder
	b.Grow(len(line))
	for c := range Clusters(line) {
		b.WriteString(c.Text)
	}
	return b.String()
}

// GraphemeRange returns the rune range [start, end) of the grapheme at col.
// At or beyond the end of line, both offsets are len(line).
func GraphemeRange(line []rune, col int) (start, end int) {
	col = max(col, 0)
	for c := range Clusters(line) {
		if col < c.End {
			return c.Start, c.End
		}
	}
	return len(line), len(line)
}

func visualWidth(line []rune) int {
	width := 0
	for c := range Clusters(line) {
		width += c.Width
	}
	return width
}

// visualWidthUntil counts terminal cells before col, excluding any grapheme
// that contains col.
func visualWidthUntil(line []rune, col int) int {
	width := 0
	for c := range Clusters(line) {
		if col < c.End {
			break
		}
		width += c.Width
	}
	return width
}

func columnForWidth(line []rune, target int) int {
	if target <= 0 {
		return 0
	}
	width := 0
	for c := range Clusters(line) {
		width += c.Width
		if width > target {
			return c.Start
		}
	}
	return len(line)
}

// sliceVisibleRunes returns whole graphemes that fit in width terminal cells.
// start must be a grapheme boundary. If the first grapheme does not fit, the
// result is empty.
func sliceVisibleRunes(line []rune, start, width int) ([]rune, int) {
	start = clamp(start, 0, len(line))
	if width <= 0 {
		return line[start:start], 0
	}
	consumed, end := 0, start
	for c := range Clusters(line[start:]) {
		if consumed+c.Width > width {
			break
		}
		consumed += c.Width
		end = start + c.End
	}
	return line[start:end], consumed
}

func visibleSegment(line []rune, offset, width int) (int, []rune, int) {
	start := columnForWidth(line, offset)
	segment, consumed := sliceVisibleRunes(line, start, width)
	return start, segment, consumed
}

// renderGrapheme applies styles without inserting escapes between runes.
// Lip Gloss v1 inserts escapes between runes for underline and strikethrough,
// which can make ANSI width calculations count one grapheme as several.
func renderGrapheme(style lipgloss.Style, text string) string {
	decorated := style.GetUnderline() || style.GetStrikethrough() ||
		style.GetUnderlineSpaces() || style.GetStrikethroughSpaces()
	if !decorated || utf8.RuneCountInString(text) == 1 {
		return style.Render(text)
	}

	// Preserve plain output when ANSI rendering is disabled.
	if rendered := style.Render(text); rendered == text {
		return rendered
	}

	decoration := ansi.NewStyle()
	if style.GetUnderline() {
		decoration = decoration.Underline()
	}
	if style.GetStrikethrough() {
		decoration = decoration.Strikethrough()
	}
	plain := style.UnsetUnderline().UnsetUnderlineSpaces().UnsetStrikethrough().UnsetStrikethroughSpaces()
	if len(decoration) == 0 {
		return plain.Render(text)
	}
	return decoration.Styled(plain.Render(text))
}
