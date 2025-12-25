// Package textarea provides a heavily changed multi-line text input component
// for Bubble Tea applications. The base was vendored from bubbles/textarea,
// but we maintain it in-tree so we can layer on features upstream does not offer:
// request-editor specific styling, line numbers, selection highlighting, viewport-aware soft wrapping,
// clipboard helpers, and other affordances we rely on for the resterm editor workflow.
package textarea

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/runeutil"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"

	"github.com/unkn0wn-root/resterm/internal/ui/scroll"
)

const (
	minHeight        = 1
	defaultHeight    = 6
	defaultWidth     = 40
	defaultCharLimit = 0 // no limit
	defaultMaxHeight = 0
	defaultMaxWidth  = 500

	// horizontalScrollMargin defines how many columns of padding we try to keep
	// between the cursor and either horizontal edge of the viewport before we
	// start shifting content. Behaves similarly to Vim's sidescrolloff.
	horizontalScrollMargin = 8

	// XXX: in v2, make max lines dynamic and default max lines configurable.
	maxLines = 10000
)

// Internal messages for clipboard operations.
type (
	pasteMsg    string
	pasteErrMsg struct{ error }
)

// RuneStyler can inject per-rune styling into the textarea during rendering.
// Implementations should return either nil (no styling) or a slice of styles
// matching the length of the provided line.
type RuneStyler interface {
	StylesForLine(line []rune, lineIndex int) []lipgloss.Style
}

// KeyMap is the key bindings for different actions within the textarea.
type KeyMap struct {
	CharacterBackward       key.Binding
	CharacterForward        key.Binding
	DeleteAfterCursor       key.Binding
	DeleteBeforeCursor      key.Binding
	DeleteCharacterBackward key.Binding
	DeleteCharacterForward  key.Binding
	DeleteWordBackward      key.Binding
	DeleteWordForward       key.Binding
	InsertNewline           key.Binding
	LineEnd                 key.Binding
	LineNext                key.Binding
	LinePrevious            key.Binding
	LineStart               key.Binding
	Paste                   key.Binding
	WordBackward            key.Binding
	WordForward             key.Binding
	InputBegin              key.Binding
	InputEnd                key.Binding

	UppercaseWordForward  key.Binding
	LowercaseWordForward  key.Binding
	CapitalizeWordForward key.Binding

	TransposeCharacterBackward key.Binding
}

// DefaultKeyMap is the default set of key bindings for navigating and acting
// upon the textarea.
var DefaultKeyMap = KeyMap{
	CharacterForward: key.NewBinding(
		key.WithKeys("right", "ctrl+f"),
		key.WithHelp("right", "character forward"),
	),
	CharacterBackward: key.NewBinding(
		key.WithKeys("left", "ctrl+b"),
		key.WithHelp("left", "character backward"),
	),
	WordForward: key.NewBinding(
		key.WithKeys("alt+right", "alt+f"),
		key.WithHelp("alt+right", "word forward"),
	),
	WordBackward: key.NewBinding(
		key.WithKeys("alt+left", "alt+b"),
		key.WithHelp("alt+left", "word backward"),
	),
	LineNext: key.NewBinding(
		key.WithKeys("down", "ctrl+n"),
		key.WithHelp("down", "next line"),
	),
	LinePrevious: key.NewBinding(
		key.WithKeys("up", "ctrl+p"),
		key.WithHelp("up", "previous line"),
	),
	DeleteWordBackward: key.NewBinding(
		key.WithKeys("alt+backspace", "ctrl+w"),
		key.WithHelp("alt+backspace", "delete word backward"),
	),
	DeleteWordForward: key.NewBinding(
		key.WithKeys("alt+delete", "alt+d"),
		key.WithHelp("alt+delete", "delete word forward"),
	),
	DeleteAfterCursor: key.NewBinding(
		key.WithKeys("ctrl+k"),
		key.WithHelp("ctrl+k", "delete after cursor"),
	),
	DeleteBeforeCursor: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "delete before cursor"),
	),
	InsertNewline: key.NewBinding(
		key.WithKeys("enter", "ctrl+m"),
		key.WithHelp("enter", "insert newline"),
	),
	DeleteCharacterBackward: key.NewBinding(
		key.WithKeys("backspace", "ctrl+h"),
		key.WithHelp("backspace", "delete character backward"),
	),
	DeleteCharacterForward: key.NewBinding(
		key.WithKeys("delete", "ctrl+d"),
		key.WithHelp("delete", "delete character forward"),
	),
	LineStart: key.NewBinding(
		key.WithKeys("home", "ctrl+a"),
		key.WithHelp("home", "line start"),
	),
	LineEnd: key.NewBinding(
		key.WithKeys("end", "ctrl+e"),
		key.WithHelp("end", "line end"),
	),
	Paste: key.NewBinding(
		key.WithKeys("ctrl+v"),
		key.WithHelp("ctrl+v", "paste"),
	),
	InputBegin: key.NewBinding(
		key.WithKeys("alt+<", "ctrl+home"),
		key.WithHelp("alt+<", "input begin"),
	),
	InputEnd: key.NewBinding(
		key.WithKeys("alt+>", "ctrl+end"),
		key.WithHelp("alt+>", "input end"),
	),

	CapitalizeWordForward: key.NewBinding(
		key.WithKeys("alt+c"),
		key.WithHelp("alt+c", "capitalize word forward"),
	),
	LowercaseWordForward: key.NewBinding(
		key.WithKeys("alt+l"),
		key.WithHelp("alt+l", "lowercase word forward"),
	),
	UppercaseWordForward: key.NewBinding(
		key.WithKeys("alt+u"),
		key.WithHelp("alt+u", "uppercase word forward"),
	),

	TransposeCharacterBackward: key.NewBinding(
		key.WithKeys("ctrl+t"),
		key.WithHelp("ctrl+t", "transpose character backward"),
	),
}

// LineInfo is a helper for keeping track of line information regarding
// soft-wrapped lines.
type LineInfo struct {
	// Width is the number of columns in the line.
	Width int
	// CharWidth is the number of characters in the line to account for
	// double-width runes.
	CharWidth int
	// Height is the number of rows in the line.
	Height int
	// StartColumn is the index of the first column of the line.
	StartColumn int
	// ColumnOffset is the number of columns that the cursor is offset from the
	// start of the line.
	ColumnOffset int
	// RowOffset is the number of rows that the cursor is offset from the start
	// of the line.
	RowOffset int
	// CharOffset is the number of characters that the cursor is offset
	// from the start of the line. This will generally be equivalent to
	// ColumnOffset, but will be different there are double-width runes before
	// the cursor.
	CharOffset int
}

// Style that will be applied to the text area.
//
// Style can be applied to focused and unfocused states to change the styles
// depending on the focus state.
//
// For an introduction to styling with Lip Gloss see:
// https://github.com/charmbracelet/lipgloss
type Style struct {
	Base             lipgloss.Style
	CursorLine       lipgloss.Style
	CursorLineNumber lipgloss.Style
	EndOfBuffer      lipgloss.Style
	LineNumber       lipgloss.Style
	Placeholder      lipgloss.Style
	Prompt           lipgloss.Style
	Text             lipgloss.Style
}

func (s Style) computedCursorLine() lipgloss.Style {
	return s.CursorLine.Inherit(s.Base).Inline(true)
}

func (s Style) computedCursorLineNumber() lipgloss.Style {
	return s.CursorLineNumber.
		Inherit(s.CursorLine).
		Inherit(s.Base).
		Inline(true)
}

func (s Style) computedEndOfBuffer() lipgloss.Style {
	return s.EndOfBuffer.Inherit(s.Base).Inline(true)
}

func (s Style) computedLineNumber() lipgloss.Style {
	return s.LineNumber.Inherit(s.Base).Inline(true)
}

func (s Style) computedPlaceholder() lipgloss.Style {
	return s.Placeholder.Inherit(s.Base).Inline(true)
}

func (s Style) computedPrompt() lipgloss.Style {
	return s.Prompt.Inherit(s.Base).Inline(true)
}

func (s Style) computedText() lipgloss.Style {
	return s.Text.Inherit(s.Base).Inline(true)
}

// Model is the Bubble Tea model for this text area element.
type Model struct {
	Err error

	// Prompt is printed at the beginning of each line.
	//
	// When changing the value of Prompt after the model has been
	// initialized, ensure that SetWidth() gets called afterwards.
	//
	// See also SetPromptFunc().
	Prompt string

	// Placeholder is the text displayed when the user
	// hasn't entered anything yet.
	Placeholder string

	// ShowLineNumbers, if enabled, causes line numbers to be printed
	// after the prompt.
	ShowLineNumbers bool

	// EndOfBufferCharacter is displayed at the end of the input.
	EndOfBufferCharacter rune

	// KeyMap encodes the keybindings recognized by the widget.
	KeyMap KeyMap

	// Styling. FocusedStyle and BlurredStyle are used to style the textarea in
	// focused and blurred states.
	FocusedStyle Style
	BlurredStyle Style
	// style is the current styling to use.
	// It is used to abstract the differences in focus state when styling the
	// model, since we can simply assign the set of styles to this variable
	// when switching focus states.
	style *Style

	selectionActive bool
	selectionStart  int
	selectionEnd    int
	selectionStyle  lipgloss.Style
	runeStyler      RuneStyler
	overlayLines    []string

	// Cursor is the text area cursor.
	Cursor cursor.Model

	// CharLimit is the maximum number of characters this input element will
	// accept. If 0 or less, there's no limit.
	CharLimit int

	// MaxHeight is the maximum height of the text area in rows. If 0 or less,
	// there's no limit.
	MaxHeight int

	// MaxWidth is the maximum width of the text area in columns. If 0 or less,
	// there's no limit.
	MaxWidth int

	// If promptFunc is set, it replaces Prompt as a generator for
	// prompt strings at the beginning of each line.
	promptFunc func(line int) string

	// promptWidth is the width of the prompt.
	promptWidth int

	// width is the maximum number of characters that can be displayed at once.
	// If 0 or less this setting is ignored.
	width int

	// height is the maximum number of lines that can be displayed at once. It
	// essentially treats the text field like a vertically scrolling viewport
	// if there are more lines than the permitted height.
	height int

	// Underlying text value.
	value [][]rune

	// focus indicates whether user input focus should be on this input
	// component. When false, ignore keyboard input and hide the cursor.
	focus bool

	// Cursor column.
	col int

	// Cursor row.
	row int

	// Last character offset, used to maintain state when the cursor is moved
	// vertically such that we can maintain the same navigating position.
	lastCharOffset int

	// viewport is the vertically-scrollable viewport of the multi-line text
	// input.
	viewport *viewport.Model

	// horizOffset tracks the first visible column of the horizontal viewport.
	horizOffset int

	// rune sanitizer for input.
	rsan runeutil.Sanitizer
}

// New creates a new model with default settings.
func New() Model {
	vp := viewport.New(0, 0)
	vp.KeyMap = viewport.KeyMap{}
	cur := cursor.New()

	focusedStyle, blurredStyle := DefaultStyles()

	m := Model{
		CharLimit:            defaultCharLimit,
		MaxHeight:            defaultMaxHeight,
		MaxWidth:             defaultMaxWidth,
		Prompt:               lipgloss.ThickBorder().Left + " ",
		style:                &blurredStyle,
		FocusedStyle:         focusedStyle,
		BlurredStyle:         blurredStyle,
		EndOfBufferCharacter: ' ',
		ShowLineNumbers:      true,
		Cursor:               cur,
		KeyMap:               DefaultKeyMap,
		selectionStyle:       lipgloss.NewStyle().Background(lipgloss.Color("#4C3F72")),

		value: make([][]rune, minHeight, maxLines),
		focus: false,
		col:   0,
		row:   0,

		viewport: &vp,
	}

	m.SetHeight(defaultHeight)
	m.SetWidth(defaultWidth)

	return m
}

// DefaultStyles returns the default styles for focused and blurred states for
// the textarea.
func DefaultStyles() (Style, Style) {
	focused := Style{
		Base: lipgloss.NewStyle(),
		CursorLine: lipgloss.NewStyle().
			Background(lipgloss.AdaptiveColor{Light: "255", Dark: "0"}),
		CursorLineNumber: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240"}),
		EndOfBuffer: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "254", Dark: "0"}),
		LineNumber: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "249", Dark: "7"}),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Text:        lipgloss.NewStyle(),
	}
	blurred := Style{
		Base: lipgloss.NewStyle(),
		CursorLine: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "7"}),
		CursorLineNumber: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "249", Dark: "7"}),
		EndOfBuffer: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "254", Dark: "0"}),
		LineNumber: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "249", Dark: "7"}),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Prompt:      lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Text: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "245", Dark: "7"}),
	}

	return focused, blurred
}

// SetValue sets the value of the text input.
func (m *Model) SetValue(s string) {
	m.Reset()
	m.InsertString(s)
}

// InsertString inserts a string at the cursor position.
func (m *Model) InsertString(s string) {
	m.insertRunesFromUserInput([]rune(s))
}

// InsertRune inserts a rune at the cursor position.
func (m *Model) InsertRune(r rune) {
	m.insertRunesFromUserInput([]rune{r})
}

// insertRunesFromUserInput inserts runes at the current cursor position.
func (m *Model) insertRunesFromUserInput(runes []rune) {
	// Clean up any special characters in the input provided by the
	// clipboard. This avoids bugs due to e.g. tab characters and
	// whatnot.
	runes = m.san().Sanitize(runes)

	if m.CharLimit > 0 {
		availSpace := m.CharLimit - m.Length()
		// If the char limit's been reached, cancel.
		if availSpace <= 0 {
			return
		}
		// If there's not enough space to paste the whole thing cut the pasted
		// runes down so they'll fit.
		if availSpace < len(runes) {
			runes = runes[:availSpace]
		}
	}

	// Split the input into lines.
	var lines [][]rune
	lstart := 0
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\n' {
			// Queue a line to become a new row in the text area below.
			// Beware to clamp the max capacity of the slice, to ensure no
			// data from different rows get overwritten when later edits
			// will modify this line.
			lines = append(lines, runes[lstart:i:i])
			lstart = i + 1
		}
	}
	if lstart <= len(runes) {
		// The last line did not end with a newline character.
		// Take it now.
		lines = append(lines, runes[lstart:])
	}

	// Obey the maximum line limit.
	if maxLines > 0 && len(m.value)+len(lines)-1 > maxLines {
		allowedHeight := max(0, maxLines-len(m.value)+1)
		lines = lines[:allowedHeight]
	}

	if len(lines) == 0 {
		// Nothing left to insert.
		return
	}

	// Save the remainder of the original line at the current
	// cursor position.
	tail := make([]rune, len(m.value[m.row][m.col:]))
	copy(tail, m.value[m.row][m.col:])

	// Paste the first line at the current cursor position.
	m.value[m.row] = append(m.value[m.row][:m.col], lines[0]...)
	m.col += len(lines[0])

	if numExtraLines := len(lines) - 1; numExtraLines > 0 {
		// Add the new lines.
		// We try to reuse the slice if there's already space.
		var newGrid [][]rune
		if cap(m.value) >= len(m.value)+numExtraLines {
			// Can reuse the extra space.
			newGrid = m.value[:len(m.value)+numExtraLines]
		} else {
			// No space left; need a new slice.
			newGrid = make([][]rune, len(m.value)+numExtraLines)
			copy(newGrid, m.value[:m.row+1])
		}
		// Add all the rows that were after the cursor in the original
		// grid at the end of the new grid.
		copy(newGrid[m.row+1+numExtraLines:], m.value[m.row+1:])
		m.value = newGrid
		// Insert all the new lines in the middle.
		for _, l := range lines[1:] {
			m.row++
			m.value[m.row] = l
			m.col = len(l)
		}
	}

	// Finally add the tail at the end of the last line inserted.
	m.value[m.row] = append(m.value[m.row], tail...)

	m.SetCursor(m.col)
}

// Value returns the value of the text input.
func (m Model) Value() string {
	if m.value == nil {
		return ""
	}

	var v strings.Builder
	for _, l := range m.value {
		v.WriteString(string(l))
		v.WriteByte('\n')
	}

	return strings.TrimSuffix(v.String(), "\n")
}

// Length returns the number of characters currently in the text input.
func (m *Model) Length() int {
	var l int
	for _, row := range m.value {
		l += uniseg.StringWidth(string(row))
	}
	// We add len(m.value) to include the newline characters.
	return l + len(m.value) - 1
}

// LineCount returns the number of lines that are currently in the text input.
func (m *Model) LineCount() int {
	return len(m.value)
}

// Line returns the line position.
func (m Model) Line() int {
	return m.row
}

// CursorDown moves the cursor down by one line.
// Returns whether or not the cursor blink should be reset.
func (m *Model) CursorDown() {
	if len(m.value) == 0 {
		return
	}

	li := m.LineInfo()
	target := m.lastCharOffset
	if target <= 0 {
		target = li.CharOffset
	}

	if m.row >= len(m.value)-1 {
		m.lastCharOffset = target
		return
	}

	m.row++
	line := m.value[m.row]
	m.col = columnForWidth(line, target)
	m.lastCharOffset = target
}

// CursorUp moves the cursor up by one line.
func (m *Model) CursorUp() {
	if len(m.value) == 0 {
		return
	}

	li := m.LineInfo()
	target := m.lastCharOffset
	if target <= 0 {
		target = li.CharOffset
	}

	if m.row <= 0 {
		m.lastCharOffset = target
		return
	}

	m.row--
	line := m.value[m.row]
	m.col = columnForWidth(line, target)
	m.lastCharOffset = target
}

// SetCursor moves the cursor to the given position. If the position is
// out of bounds the cursor will be moved to the start or end accordingly.
func (m *Model) SetCursor(col int) {
	m.col = clamp(col, 0, len(m.value[m.row]))
	// Any time that we move the cursor horizontally we need to reset the last
	// offset so that the horizontal position when navigating is adjusted.
	m.lastCharOffset = 0
	m.repositionHorizontal()
}

// CursorStart moves the cursor to the start of the input field.
func (m *Model) CursorStart() {
	m.SetCursor(0)
}

// CursorEnd moves the cursor to the end of the input field.
func (m *Model) CursorEnd() {
	m.SetCursor(len(m.value[m.row]))
}

// Focused returns the focus state on the model.
func (m Model) Focused() bool {
	return m.focus
}

// Focus sets the focus state on the model. When the model is in focus it can
// receive keyboard input and the cursor will be hidden.
func (m *Model) Focus() tea.Cmd {
	m.focus = true
	m.style = &m.FocusedStyle
	return m.Cursor.Focus()
}

// Blur removes the focus state on the model. When the model is blurred it can
// not receive keyboard input and the cursor will be hidden.
func (m *Model) Blur() {
	m.focus = false
	m.style = &m.BlurredStyle
	m.Cursor.Blur()
}

// Reset sets the input to its default state with no input.
func (m *Model) Reset() {
	m.value = make([][]rune, minHeight, maxLines)
	m.col = 0
	m.row = 0
	m.horizOffset = 0
	m.viewport.GotoTop()
	m.SetCursor(0)
}

// san initializes or retrieves the rune sanitizer.
func (m *Model) san() runeutil.Sanitizer {
	if m.rsan == nil {
		// Textinput has all its input on a single line so collapse
		// newlines/tabs to single spaces.
		m.rsan = runeutil.NewSanitizer()
	}
	return m.rsan
}

// deleteBeforeCursor deletes all text before the cursor. Returns whether or
// not the cursor blink should be reset.
func (m *Model) deleteBeforeCursor() {
	m.value[m.row] = m.value[m.row][m.col:]
	m.SetCursor(0)
}

// deleteAfterCursor deletes all text after the cursor. Returns whether or not
// the cursor blink should be reset. If input is masked delete everything after
// the cursor so as not to reveal word breaks in the masked input.
func (m *Model) deleteAfterCursor() {
	m.value[m.row] = m.value[m.row][:m.col]
	m.SetCursor(len(m.value[m.row]))
}

// transposeLeft exchanges the runes at the cursor and immediately
// before. No-op if the cursor is at the beginning of the line.  If
// the cursor is not at the end of the line yet, moves the cursor to
// the right.
func (m *Model) transposeLeft() {
	if m.col == 0 || len(m.value[m.row]) < 2 {
		return
	}
	if m.col >= len(m.value[m.row]) {
		m.SetCursor(m.col - 1)
	}
	m.value[m.row][m.col-1], m.value[m.row][m.col] = m.value[m.row][m.col], m.value[m.row][m.col-1]
	if m.col < len(m.value[m.row]) {
		m.SetCursor(m.col + 1)
	}
}

// deleteWordLeft deletes the word left to the cursor. Returns whether or not
// the cursor blink should be reset.
func (m *Model) deleteWordLeft() {
	if m.col == 0 || len(m.value[m.row]) == 0 {
		return
	}

	// Linter note: it's critical that we acquire the initial cursor position
	// here prior to altering it via SetCursor() below. As such, moving this
	// call into the corresponding if clause does not apply here.
	oldCol := m.col

	m.SetCursor(m.col - 1)
	for unicode.IsSpace(m.value[m.row][m.col]) {
		if m.col <= 0 {
			break
		}
		// ignore series of whitespace before cursor
		m.SetCursor(m.col - 1)
	}

	for m.col > 0 {
		if !unicode.IsSpace(m.value[m.row][m.col]) {
			m.SetCursor(m.col - 1)
		} else {
			if m.col > 0 {
				// keep the previous space
				m.SetCursor(m.col + 1)
			}
			break
		}
	}

	if oldCol > len(m.value[m.row]) {
		m.value[m.row] = m.value[m.row][:m.col]
	} else {
		m.value[m.row] = append(m.value[m.row][:m.col], m.value[m.row][oldCol:]...)
	}
}

// deleteWordRight deletes the word right to the cursor.
func (m *Model) deleteWordRight() {
	if m.col >= len(m.value[m.row]) || len(m.value[m.row]) == 0 {
		return
	}

	oldCol := m.col

	for m.col < len(m.value[m.row]) && unicode.IsSpace(m.value[m.row][m.col]) {
		// ignore series of whitespace after cursor
		m.SetCursor(m.col + 1)
	}

	for m.col < len(m.value[m.row]) {
		if !unicode.IsSpace(m.value[m.row][m.col]) {
			m.SetCursor(m.col + 1)
		} else {
			break
		}
	}

	if m.col > len(m.value[m.row]) {
		m.value[m.row] = m.value[m.row][:oldCol]
	} else {
		m.value[m.row] = append(m.value[m.row][:oldCol], m.value[m.row][m.col:]...)
	}

	m.SetCursor(oldCol)
}

// characterRight moves the cursor one character to the right.
func (m *Model) characterRight() {
	if m.col < len(m.value[m.row]) {
		m.SetCursor(m.col + 1)
	} else {
		if m.row < len(m.value)-1 {
			m.row++
			m.CursorStart()
		}
	}
}

// characterLeft moves the cursor one character to the left.
// If insideLine is set, the cursor is moved to the last
// character in the previous line, instead of one past that.
func (m *Model) characterLeft(insideLine bool) {
	if m.col == 0 && m.row != 0 {
		m.row--
		m.CursorEnd()
		if !insideLine {
			return
		}
	}
	if m.col > 0 {
		m.SetCursor(m.col - 1)
	}
}

// wordLeft moves the cursor one word to the left. Returns whether or not the
// cursor blink should be reset. If input is masked, move input to the start
// so as not to reveal word breaks in the masked input.
func (m *Model) wordLeft() {
	for {
		m.characterLeft(true /* insideLine */)
		if m.col < len(m.value[m.row]) && !unicode.IsSpace(m.value[m.row][m.col]) {
			break
		}
	}

	for m.col > 0 {
		if unicode.IsSpace(m.value[m.row][m.col-1]) {
			break
		}
		m.SetCursor(m.col - 1)
	}
}

// wordRight moves the cursor one word to the right. Returns whether or not the
// cursor blink should be reset. If the input is masked, move input to the end
// so as not to reveal word breaks in the masked input.
func (m *Model) wordRight() {
	m.doWordRight(func(int, int) { /* nothing */ })
}

func (m *Model) doWordRight(fn func(charIdx int, pos int)) {
	// Skip spaces forward.
	for m.col >= len(m.value[m.row]) || unicode.IsSpace(m.value[m.row][m.col]) {
		if m.row == len(m.value)-1 && m.col == len(m.value[m.row]) {
			// End of text.
			break
		}
		m.characterRight()
	}

	charIdx := 0
	for m.col < len(m.value[m.row]) {
		if unicode.IsSpace(m.value[m.row][m.col]) {
			break
		}
		fn(charIdx, m.col)
		m.SetCursor(m.col + 1)
		charIdx++
	}
}

// uppercaseRight changes the word to the right to uppercase.
func (m *Model) uppercaseRight() {
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToUpper(m.value[m.row][i])
	})
}

// lowercaseRight changes the word to the right to lowercase.
func (m *Model) lowercaseRight() {
	m.doWordRight(func(_ int, i int) {
		m.value[m.row][i] = unicode.ToLower(m.value[m.row][i])
	})
}

// capitalizeRight changes the word to the right to title case.
func (m *Model) capitalizeRight() {
	m.doWordRight(func(charIdx int, i int) {
		if charIdx == 0 {
			m.value[m.row][i] = unicode.ToTitle(m.value[m.row][i])
		}
	})
}

// LineInfo describes the cursor's position within the current line.
func (m Model) LineInfo() LineInfo {
	if len(m.value) == 0 || m.row < 0 || m.row >= len(m.value) {
		return LineInfo{}
	}

	line := m.value[m.row]
	charWidth := visualWidth(line)
	charOffset := visualWidthUntil(line, m.col)

	return LineInfo{
		Width:        len(line),
		CharWidth:    charWidth,
		Height:       1,
		StartColumn:  0,
		ColumnOffset: m.col,
		RowOffset:    0,
		CharOffset:   charOffset,
	}
}

// SetSelectionRange marks the inclusive-exclusive rune offsets that should be
// highlighted in the rendered view. Passing identical offsets clears the
// highlight.
func (m *Model) SetSelectionRange(start, end int) {
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	total := m.Length()
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	if start == end {
		m.selectionActive = false
		m.selectionStart = 0
		m.selectionEnd = 0
		return
	}
	m.selectionActive = true
	m.selectionStart = start
	m.selectionEnd = end
}

// ClearSelectionRange removes any active highlight.
func (m *Model) ClearSelectionRange() {
	m.selectionActive = false
	m.selectionStart = 0
	m.selectionEnd = 0
}

// SelectionStyle returns the style used to paint the active selection.
func (m Model) SelectionStyle() lipgloss.Style {
	return m.selectionStyle
}

// SetSelectionStyle overrides the default highlight style.
func (m *Model) SetSelectionStyle(style lipgloss.Style) {
	m.selectionStyle = style
}

// SetOverlayLines configures auxiliary lines inserted after the cursor row.
func (m *Model) SetOverlayLines(lines []string) {
	if len(lines) == 0 {
		m.overlayLines = nil
		return
	}
	buffer := make([]string, len(lines))
	copy(buffer, lines)
	m.overlayLines = buffer
}

// ClearOverlay clears any active overlay content.
func (m *Model) ClearOverlay() {
	m.overlayLines = nil
}

// SetRuneStyler registers a styling hook applied when rendering each line.
func (m *Model) SetRuneStyler(styler RuneStyler) {
	m.runeStyler = styler
}

// RuneStyler returns the styling hook for per-rune rendering, if any.
func (m Model) RuneStyler() RuneStyler {
	return m.runeStyler
}

// ViewStart returns the index of the first visible line in the viewport.
func (m Model) ViewStart() int {
	if m.viewport == nil {
		return 0
	}
	return m.viewport.YOffset
}

// SetViewStart updates the viewport so that the line at offset becomes the first visible line.
func (m *Model) SetViewStart(offset int) {
	if m.viewport == nil {
		return
	}
	// Ensure the viewport has up-to-date content before applying the offset so
	// that the y-offset clamps against the real scroll bounds. Without this the
	// viewport may believe it has zero lines, causing any non-zero offset to be
	// clamped back to zero.
	//
	// Calling View() is enough to refresh the viewport's internal line buffer.
	// We ignore the rendered output here because we only need the side effect of
	// populating the viewport state used for clamping.
	_ = m.View()
	m.viewport.SetYOffset(offset)
}

// repositionView repositions the view of the viewport based on the defined
// scrolling behavior.
func (m *Model) repositionView() {
	row := m.cursorLineNumber()
	h := m.viewport.Height
	if h <= 0 {
		h = m.height
	}
	total := len(m.value)
	if total < 1 {
		total = 1
	}
	maxOff := total - h
	if maxOff < 0 {
		maxOff = 0
	}
	target := scroll.Align(row, m.viewport.YOffset, h, total)
	if m.row >= len(m.value)-1 {
		target = maxOff
	}
	if target != m.viewport.YOffset {
		if target < 0 {
			target = 0
		}
		if target > maxOff {
			target = maxOff
		}
		m.viewport.YOffset = target
	}
}

func (m *Model) repositionHorizontal() {
	if m.width <= 0 {
		m.horizOffset = 0
		return
	}
	if len(m.value) == 0 || m.row < 0 || m.row >= len(m.value) {
		m.horizOffset = 0
		return
	}

	line := m.value[m.row]

	margin := horizontalScrollMargin
	if m.width > 0 {
		maxMargin := max(0, (m.width-1)/2)
		if margin > maxMargin {
			margin = maxMargin
		}
	} else {
		margin = 0
	}

	lineWidth := visualWidth(line)
	if lineWidth <= m.width {
		margin = 0
	}
	if m.width <= horizontalScrollMargin+1 {
		margin = 0
	}
	maxOffset := max(0, lineWidth+margin-m.width)

	cursorLeft := visualWidthUntil(line, m.col)
	cursorWidth := 1
	if m.col < len(line) {
		cursorWidth = safeRuneWidth(line[m.col])
	}

	leftBoundary := m.horizOffset + margin
	rightBoundary := m.horizOffset + m.width - margin

	if cursorLeft < leftBoundary {
		m.horizOffset = cursorLeft - margin
	} else if cursorLeft+cursorWidth > rightBoundary {
		m.horizOffset = cursorLeft + cursorWidth + margin - m.width
	}

	if m.horizOffset > maxOffset {
		m.horizOffset = maxOffset
	}
	if m.horizOffset < 0 {
		m.horizOffset = 0
	}

	startIdx := columnForWidth(line, m.horizOffset)
	m.horizOffset = visualWidthUntil(line, startIdx)
}

// Width returns the width of the textarea.
func (m Model) Width() int {
	return m.width
}

// moveToBegin moves the cursor to the beginning of the input.
func (m *Model) moveToBegin() {
	m.row = 0
	m.SetCursor(0)
}

// moveToEnd moves the cursor to the end of the input.
func (m *Model) moveToEnd() {
	m.row = len(m.value) - 1
	m.SetCursor(len(m.value[m.row]))
}

// SetWidth sets the width of the textarea to fit exactly within the given width.
// This means that the textarea will account for the width of the prompt and
// whether or not line numbers are being shown.
//
// Ensure that SetWidth is called after setting the Prompt and ShowLineNumbers,
// It is important that the width of the textarea be exactly the given width
// and no more.
func (m *Model) SetWidth(w int) {
	// Update prompt width only if there is no prompt function as SetPromptFunc
	// updates the prompt width when it is called.
	if m.promptFunc == nil {
		m.promptWidth = uniseg.StringWidth(m.Prompt)
	}

	// Add base style borders and padding to reserved outer width.
	reservedOuter := m.style.Base.GetHorizontalFrameSize()

	// Add prompt width to reserved inner width.
	reservedInner := m.promptWidth

	// Add line number width to reserved inner width.
	if m.ShowLineNumbers {
		const lnWidth = 4 // Up to 3 digits for line number plus 1 margin.
		reservedInner += lnWidth
	}

	// Input width must be at least one more than the reserved inner and outer
	// width. This gives us a minimum input width of 1.
	minWidth := reservedInner + reservedOuter + 1
	inputWidth := max(w, minWidth)

	// Input width must be no more than maximum width.
	if m.MaxWidth > 0 {
		inputWidth = min(inputWidth, m.MaxWidth)
	}

	// Since the width of the viewport and input area is dependent on the width of
	// borders, prompt and line numbers, we need to calculate it by subtracting
	// the reserved width from them.

	m.viewport.Width = inputWidth - reservedOuter
	m.width = inputWidth - reservedOuter - reservedInner
	m.repositionHorizontal()
}

// SetPromptFunc supersedes the Prompt field and sets a dynamic prompt
// instead.
// If the function returns a prompt that is shorter than the
// specified promptWidth, it will be padded to the left.
// If it returns a prompt that is longer, display artifacts
// may occur; the caller is responsible for computing an adequate
// promptWidth.
func (m *Model) SetPromptFunc(promptWidth int, fn func(lineIdx int) string) {
	m.promptFunc = fn
	m.promptWidth = promptWidth
}

// Height returns the current height of the textarea.
func (m Model) Height() int {
	return m.height
}

// SetHeight sets the height of the textarea.
func (m *Model) SetHeight(h int) {
	if m.MaxHeight > 0 {
		m.height = clamp(h, minHeight, m.MaxHeight)
		m.viewport.Height = clamp(h, minHeight, m.MaxHeight)
	} else {
		m.height = max(h, minHeight)
		m.viewport.Height = max(h, minHeight)
	}
}

// Update is the Bubble Tea update loop.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focus {
		m.Cursor.Blur()
		return m, nil
	}

	// Used to determine if the cursor should blink.
	oldRow, oldCol := m.cursorLineNumber(), m.col

	var cmds []tea.Cmd

	if m.value[m.row] == nil {
		m.value[m.row] = make([]rune, 0)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.KeyMap.DeleteAfterCursor):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
			m.deleteAfterCursor()
		case key.Matches(msg, m.KeyMap.DeleteBeforeCursor):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			m.deleteBeforeCursor()
		case key.Matches(msg, m.KeyMap.DeleteCharacterBackward):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			if len(m.value[m.row]) > 0 {
				m.value[m.row] = append(m.value[m.row][:max(0, m.col-1)], m.value[m.row][m.col:]...)
				if m.col > 0 {
					m.SetCursor(m.col - 1)
				}
			}
		case key.Matches(msg, m.KeyMap.DeleteCharacterForward):
			if len(m.value[m.row]) > 0 && m.col < len(m.value[m.row]) {
				m.value[m.row] = append(m.value[m.row][:m.col], m.value[m.row][m.col+1:]...)
			}
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
		case key.Matches(msg, m.KeyMap.DeleteWordBackward):
			if m.col <= 0 {
				m.mergeLineAbove(m.row)
				break
			}
			m.deleteWordLeft()
		case key.Matches(msg, m.KeyMap.DeleteWordForward):
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			if m.col >= len(m.value[m.row]) {
				m.mergeLineBelow(m.row)
				break
			}
			m.deleteWordRight()
		case key.Matches(msg, m.KeyMap.InsertNewline):
			if m.MaxHeight > 0 && len(m.value) >= m.MaxHeight {
				return m, nil
			}
			m.col = clamp(m.col, 0, len(m.value[m.row]))
			m.splitLine(m.row, m.col)
		case key.Matches(msg, m.KeyMap.LineEnd):
			m.CursorEnd()
		case key.Matches(msg, m.KeyMap.LineStart):
			m.CursorStart()
		case key.Matches(msg, m.KeyMap.CharacterForward):
			m.characterRight()
		case key.Matches(msg, m.KeyMap.LineNext):
			m.CursorDown()
		case key.Matches(msg, m.KeyMap.WordForward):
			m.wordRight()
		case key.Matches(msg, m.KeyMap.Paste):
			return m, Paste
		case key.Matches(msg, m.KeyMap.CharacterBackward):
			m.characterLeft(false /* insideLine */)
		case key.Matches(msg, m.KeyMap.LinePrevious):
			m.CursorUp()
		case key.Matches(msg, m.KeyMap.WordBackward):
			m.wordLeft()
		case key.Matches(msg, m.KeyMap.InputBegin):
			m.moveToBegin()
		case key.Matches(msg, m.KeyMap.InputEnd):
			m.moveToEnd()
		case key.Matches(msg, m.KeyMap.LowercaseWordForward):
			m.lowercaseRight()
		case key.Matches(msg, m.KeyMap.UppercaseWordForward):
			m.uppercaseRight()
		case key.Matches(msg, m.KeyMap.CapitalizeWordForward):
			m.capitalizeRight()
		case key.Matches(msg, m.KeyMap.TransposeCharacterBackward):
			m.transposeLeft()

		default:
			m.insertRunesFromUserInput(msg.Runes)
		}

	case pasteMsg:
		m.insertRunesFromUserInput([]rune(msg))

	case pasteErrMsg:
		m.Err = msg
	}

	vp, cmd := m.viewport.Update(msg)
	m.viewport = &vp
	cmds = append(cmds, cmd)

	newRow, newCol := m.cursorLineNumber(), m.col
	m.Cursor, cmd = m.Cursor.Update(msg)
	if (newRow != oldRow || newCol != oldCol) && m.Cursor.Mode() == cursor.CursorBlink {
		m.Cursor.Blink = false
		cmd = m.Cursor.BlinkCmd()
	}
	cmds = append(cmds, cmd)

	m.repositionView()
	m.repositionHorizontal()

	return m, tea.Batch(cmds...)
}

// View renders the text area in its current state.
func (m Model) View() string {
	if m.Value() == "" && m.row == 0 && m.col == 0 && m.Placeholder != "" {
		return m.placeholderView()
	}
	m.Cursor.TextStyle = m.style.computedCursorLine()

	selectionActive := m.selectionActive && m.selectionEnd > m.selectionStart
	selStart := m.selectionStart
	selEnd := m.selectionEnd
	globalOffset := 0

	var (
		s                strings.Builder
		widestLineNumber int
	)

	overlayLines := m.overlayLines
	overlayActive := len(overlayLines) > 0

	viewPad := 3
	visibleStart := 0
	visibleEnd := m.height + viewPad
	if m.viewport != nil {
		viewTop := m.viewport.YOffset
		viewHeight := m.viewport.Height
		if viewHeight <= 0 {
			viewHeight = m.height
		}
		visibleStart = max(viewTop-viewPad, 0)
		visibleEnd = viewTop + viewHeight + viewPad
	}

	displayLine := 0
	for l, line := range m.value {
		currentRow := displayLine
		displayLine++

		lineLen := len(line)
		lineStartOffset := globalOffset
		lineEndOffset := lineStartOffset + lineLen
		newlineSelected := selectionActive && l < len(m.value)-1 && selStart <= lineEndOffset &&
			lineEndOffset < selEnd

		lineVisible := currentRow >= visibleStart && currentRow <= visibleEnd

		var lineStyles []lipgloss.Style
		if lineVisible && m.runeStyler != nil && len(line) > 0 {
			if styles := m.runeStyler.StylesForLine(line, l); len(styles) == len(line) {
				lineStyles = styles
			}
		}

		var style lipgloss.Style
		if m.row == l {
			style = m.style.computedCursorLine()
		} else {
			style = m.style.computedText()
		}

		if !lineVisible {
			globalOffset += lineLen
			if l < len(m.value)-1 {
				globalOffset++
			}
			s.WriteRune('\n')
			continue
		}

		prompt := m.getPromptString(currentRow)
		prompt = m.style.computedPrompt().Render(prompt)
		s.WriteString(style.Render(prompt))

		var ln string
		if m.ShowLineNumbers {
			if m.row == l {
				ln = style.Render(
					m.style.computedCursorLineNumber().Render(m.formatLineNumber(l + 1)),
				)
			} else {
				ln = style.Render(m.style.computedLineNumber().Render(m.formatLineNumber(l + 1)))
			}
			s.WriteString(ln)
			lnw := lipgloss.Width(ln)
			if lnw > widestLineNumber {
				widestLineNumber = lnw
			}
		}

		startIdx, visibleRunes, renderedWidth := visibleSegment(line, m.horizOffset, m.width)
		lineConsumed := startIdx
		globalOffset += startIdx
		needsStyler := lineStyles != nil || selectionActive

		cursorRel := m.col - startIdx
		cursorVisible := m.row == l && cursorRel >= 0 && cursorRel <= len(visibleRunes)

		if !needsStyler {
			if cursorVisible {
				beforeEnd := min(cursorRel, len(visibleRunes))
				if beforeEnd > 0 {
					s.WriteString(style.Render(string(visibleRunes[:beforeEnd])))
				}
				if cursorRel < len(visibleRunes) {
					m.Cursor.SetChar(string(visibleRunes[cursorRel]))
					s.WriteString(style.Render(m.Cursor.View()))
					if cursorRel+1 < len(visibleRunes) {
						s.WriteString(style.Render(string(visibleRunes[cursorRel+1:])))
					}
				} else {
					m.Cursor.SetChar(" ")
					s.WriteString(style.Render(m.Cursor.View()))
				}
			} else {
				s.WriteString(style.Render(string(visibleRunes)))
			}
			lineConsumed += len(visibleRunes)
			globalOffset += len(visibleRunes)
		} else {
			segmentStart := lineConsumed
			segments := m.renderStyledSegments(
				visibleRunes,
				style,
				lineStyles,
				&lineConsumed,
				lineLen,
				&globalOffset,
				selectionActive,
				selStart,
				selEnd,
			)

			if cursorVisible {
				writeSegments(&s, segments, 0, min(cursorRel, len(segments)))
				if cursorRel < len(visibleRunes) {
					m.Cursor.SetChar(string(visibleRunes[cursorRel]))
					cursorStyle := style
					cursorIndex := segmentStart + cursorRel
					if lineStyles != nil && cursorIndex >= 0 && cursorIndex < len(lineStyles) {
						cursorStyle = cursorStyle.Inherit(lineStyles[cursorIndex])
					}
					s.WriteString(cursorStyle.Render(m.Cursor.View()))
					writeSegments(&s, segments, cursorRel+1, len(segments))
				} else {
					m.Cursor.SetChar(" ")
					s.WriteString(style.Render(m.Cursor.View()))
				}
			} else {
				writeSegments(&s, segments, 0, len(segments))
			}
		}

		if remaining := lineLen - lineConsumed; remaining > 0 {
			lineConsumed += remaining
			globalOffset += remaining
		}

		pad := strings.Repeat(" ", max(0, m.width-renderedWidth))
		if selectionActive && newlineSelected && pad != "" {
			newlineStyle := m.selectionStyle.Inherit(style)
			s.WriteString(newlineStyle.Render(pad))
		} else {
			s.WriteString(style.Render(pad))
		}

		s.WriteRune('\n')

		if overlayActive && m.row == l {
			displayLine, widestLineNumber = m.renderOverlayLines(
				&s,
				displayLine,
				widestLineNumber,
				overlayLines,
			)
			overlayActive = false
		}

		if l < len(m.value)-1 {
			globalOffset++
		}
	}

	if overlayActive {
		displayLine, widestLineNumber = m.renderOverlayLines(
			&s,
			displayLine,
			widestLineNumber,
			overlayLines,
		)
	}

	for i := 0; i < m.height; i++ {
		prompt := m.getPromptString(displayLine)
		prompt = m.style.computedPrompt().Render(prompt)
		s.WriteString(prompt)
		displayLine++

		leftGutter := string(m.EndOfBufferCharacter)
		rightGapWidth := m.Width() - lipgloss.Width(leftGutter) + widestLineNumber
		rightGap := strings.Repeat(" ", max(0, rightGapWidth))
		s.WriteString(m.style.computedEndOfBuffer().Render(leftGutter + rightGap))
		s.WriteRune('\n')
	}

	m.viewport.SetContent(s.String())
	return m.style.Base.Render(m.viewport.View())
}

func (m *Model) renderOverlayLines(
	builder *strings.Builder,
	displayLine int,
	widestLineNumber int,
	lines []string,
) (int, int) {
	if len(lines) == 0 {
		return displayLine, widestLineNumber
	}
	textStyle := m.style.computedText()
	for _, line := range lines {
		prompt := m.getPromptString(displayLine)
		prompt = m.style.computedPrompt().Render(prompt)
		builder.WriteString(textStyle.Render(prompt))

		var ln string
		if m.ShowLineNumbers {
			ln = textStyle.Render(m.style.computedLineNumber().Render(m.formatLineNumber(" ")))
			builder.WriteString(ln)
			lnw := lipgloss.Width(ln)
			if lnw > widestLineNumber {
				widestLineNumber = lnw
			}
		}

		visible := line
		if m.horizOffset > 0 || lipgloss.Width(line) > m.width {
			left := m.horizOffset
			right := left + m.width
			visible = ansi.Cut(line, left, right)
		}
		renderedWidth := lipgloss.Width(visible)
		builder.WriteString(textStyle.Render(visible))
		padWidth := max(0, m.width-renderedWidth)
		if padWidth > 0 {
			pad := strings.Repeat(" ", padWidth)
			builder.WriteString(textStyle.Render(pad))
		}
		builder.WriteRune('\n')
		displayLine++
	}
	return displayLine, widestLineNumber
}

func writeSegments(builder *strings.Builder, segments []string, start, end int) {
	if start < 0 {
		start = 0
	}
	if end > len(segments) {
		end = len(segments)
	}
	if start >= end {
		return
	}
	for i := start; i < end; i++ {
		builder.WriteString(segments[i])
	}
}

func (m Model) renderStyledSegments(
	wrappedLine []rune,
	baseStyle lipgloss.Style,
	lineStyles []lipgloss.Style,
	lineConsumed *int,
	lineLen int,
	globalOffset *int,
	selectionActive bool,
	selectionStart, selectionEnd int,
) []string {
	segments := make([]string, len(wrappedLine))
	for i, r := range wrappedLine {
		isActual := *lineConsumed < lineLen
		runeStyle := baseStyle
		if isActual && lineStyles != nil {
			idx := *lineConsumed
			if idx >= 0 && idx < len(lineStyles) {
				runeStyle = runeStyle.Inherit(lineStyles[idx])
			}
		}

		renderStyle := runeStyle
		if selectionActive && isActual && *globalOffset >= selectionStart &&
			*globalOffset < selectionEnd {
			renderStyle = m.selectionStyle.Inherit(runeStyle)
		}

		segments[i] = renderStyle.Render(string(r))
		if isActual {
			*lineConsumed++
			*globalOffset++
		}
	}
	return segments
}

// formatLineNumber formats the line number for display dynamically based on
// the maximum number of lines.
func (m Model) formatLineNumber(x any) string {
	maxLine := len(m.value)
	if maxLine < 1 {
		maxLine = 1
	}
	if m.height > maxLine {
		maxLine = m.height
	}
	if v, ok := x.(int); ok && v > maxLine {
		maxLine = v
	} else if s, ok := x.(string); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > maxLine {
			maxLine = n
		}
	}
	digits := len(strconv.Itoa(maxLine))
	if digits < 2 {
		digits = 2
	}
	return fmt.Sprintf(" %*v ", digits, x)
}

func (m Model) getPromptString(displayLine int) (prompt string) {
	prompt = m.Prompt
	if m.promptFunc == nil {
		return prompt
	}
	prompt = m.promptFunc(displayLine)
	pl := uniseg.StringWidth(prompt)
	if pl < m.promptWidth {
		prompt = fmt.Sprintf("%*s%s", m.promptWidth-pl, "", prompt)
	}
	return prompt
}

// placeholderView returns the prompt and placeholder view, if any.
func (m Model) placeholderView() string {
	var (
		s     strings.Builder
		p     = m.Placeholder
		style = m.style.computedPlaceholder()
	)

	// word wrap lines
	pwordwrap := ansi.Wordwrap(p, m.width, "")
	// wrap lines (handles lines that could not be word wrapped)
	pwrap := ansi.Hardwrap(pwordwrap, m.width, true)
	// split string by new lines
	plines := strings.Split(strings.TrimSpace(pwrap), "\n")

	for i := 0; i < m.height; i++ {
		lineStyle := m.style.computedPlaceholder()
		lineNumberStyle := m.style.computedLineNumber()
		if len(plines) > i {
			lineStyle = m.style.computedCursorLine()
			lineNumberStyle = m.style.computedCursorLineNumber()
		}

		// render prompt
		prompt := m.getPromptString(i)
		prompt = m.style.computedPrompt().Render(prompt)
		s.WriteString(lineStyle.Render(prompt))

		// when show line numbers enabled:
		// - render line number for only the cursor line
		// - indent other placeholder lines
		// this is consistent with vim with line numbers enabled
		if m.ShowLineNumbers {
			var ln string

			switch {
			case i == 0:
				ln = strconv.Itoa(i + 1)
				fallthrough
			case len(plines) > i:
				s.WriteString(lineStyle.Render(lineNumberStyle.Render(m.formatLineNumber(ln))))
			default:
			}
		}

		switch {
		// first line
		case i == 0:
			// first character of first line as cursor with character
			m.Cursor.TextStyle = m.style.computedPlaceholder()

			ch, rest, _, _ := uniseg.FirstGraphemeClusterInString(plines[0], 0)
			m.Cursor.SetChar(ch)
			s.WriteString(lineStyle.Render(m.Cursor.View()))

			// the rest of the first line
			s.WriteString(lineStyle.Render(style.Render(rest)))
		// remaining lines
		case len(plines) > i:
			// current line placeholder text
			if len(plines) > i {
				s.WriteString(
					lineStyle.Render(
						style.Render(
							plines[i] + strings.Repeat(
								" ",
								max(0, m.width-uniseg.StringWidth(plines[i])),
							),
						),
					),
				)
			}
		default:
			// end of line buffer character
			eob := m.style.computedEndOfBuffer().Render(string(m.EndOfBufferCharacter))
			s.WriteString(eob)
		}

		// terminate with new line
		s.WriteRune('\n')
	}

	m.viewport.SetContent(s.String())
	return m.style.Base.Render(m.viewport.View())
}

// Blink returns the blink command for the cursor.
func Blink() tea.Msg {
	return cursor.Blink()
}

// cursorLineNumber returns the line number that the cursor is on.
func (m Model) cursorLineNumber() int {
	if len(m.value) == 0 {
		return 0
	}
	if m.row < 0 {
		return 0
	}
	if m.row >= len(m.value) {
		return len(m.value) - 1
	}
	return m.row
}

// mergeLineBelow merges the current line the cursor is on with the line below.
func (m *Model) mergeLineBelow(row int) {
	if row >= len(m.value)-1 {
		return
	}

	// To perform a merge, we will need to combine the two lines and then
	m.value[row] = append(m.value[row], m.value[row+1]...)

	// Shift all lines up by one
	for i := row + 1; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}

	// And, remove the last line
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

// mergeLineAbove merges the current line the cursor is on with the line above.
func (m *Model) mergeLineAbove(row int) {
	if row <= 0 {
		return
	}

	m.col = len(m.value[row-1])
	m.row = m.row - 1

	// To perform a merge, we will need to combine the two lines and then
	m.value[row-1] = append(m.value[row-1], m.value[row]...)

	// Shift all lines up by one
	for i := row; i < len(m.value)-1; i++ {
		m.value[i] = m.value[i+1]
	}

	// And, remove the last line
	if len(m.value) > 0 {
		m.value = m.value[:len(m.value)-1]
	}
}

func (m *Model) splitLine(row, col int) {
	// To perform a split, take the current line and keep the content before
	// the cursor, take the content after the cursor and make it the content of
	// the line underneath, and shift the remaining lines down by one
	head, tailSrc := m.value[row][:col], m.value[row][col:]
	tail := make([]rune, len(tailSrc))
	copy(tail, tailSrc)

	m.value = append(m.value[:row+1], m.value[row:]...)

	m.value[row] = head
	m.value[row+1] = tail

	m.col = 0
	m.row++
}

// Paste is a command for pasting from the clipboard into the text input.
func Paste() tea.Msg {
	str, err := clipboard.ReadAll()
	if err != nil {
		return pasteErrMsg{err}
	}
	return pasteMsg(str)
}

func visualWidth(runes []rune) int {
	width := 0
	for _, r := range runes {
		width += rw.RuneWidth(r)
	}
	return width
}

func safeRuneWidth(r rune) int {
	if w := rw.RuneWidth(r); w > 0 {
		return w
	}
	return 1
}

func visualWidthUntil(runes []rune, col int) int {
	if col <= 0 {
		return 0
	}
	if col > len(runes) {
		col = len(runes)
	}
	return visualWidth(runes[:col])
}

func columnForWidth(runes []rune, target int) int {
	if target <= 0 {
		return 0
	}
	width := 0
	for i, r := range runes {
		width += rw.RuneWidth(r)
		if width > target {
			return i
		}
	}
	return len(runes)
}

func sliceVisibleRunes(line []rune, start, width int) ([]rune, int) {
	if start < 0 {
		start = 0
	}
	if start > len(line) {
		start = len(line)
	}
	if width <= 0 {
		return line[start:start], 0
	}
	consumed := 0
	end := start
	for end < len(line) {
		w := rw.RuneWidth(line[end])
		if consumed+w > width && end > start {
			break
		}
		consumed += w
		end++
		if consumed >= width {
			break
		}
	}
	return line[start:end], consumed
}

func visibleSegment(line []rune, offset, width int) (int, []rune, int) {
	start := columnForWidth(line, offset)
	segment, consumed := sliceVisibleRunes(line, start, width)
	return start, segment, consumed
}

func clamp(v, low, high int) int {
	if high < low {
		low, high = high, low
	}
	return min(high, max(low, v))
}
