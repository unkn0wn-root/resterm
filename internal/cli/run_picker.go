package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
	str "github.com/unkn0wn-root/resterm/internal/util"
	"golang.org/x/term"
)

const (
	runRequestPickerDefaultWidth = 68
	runRequestPickerMaxWidth     = 76
	runRequestPickerDefaultRows  = 8
	runRequestPickerMinRows      = 4
	runRequestPickerMaxRows      = 12
	runRequestPickerCompactWidth = 44
	runRequestPickerSideMargin   = 2
	runRequestPickerChromeRows   = 10

	runRequestPickerColBorder        = "#4B4670"
	runRequestPickerColTitle         = "#F4E27A"
	runRequestPickerColPath          = "#D2D4F5"
	runRequestPickerColMuted         = "#8E88A7"
	runRequestPickerColSelBg         = "#261F42"
	runRequestPickerColSelFg         = "#F8F7FF"
	runRequestPickerColCursor        = "#6F688D"
	runRequestPickerColAccent        = "#FFD46A"
	runRequestPickerColText          = "#E8E9F0"
	runRequestPickerColBright        = "#FFFFFF"
	runRequestPickerColLineSel       = "#C8C2E6"
	runRequestPickerColTarget        = "#7DB9FF"
	runRequestPickerColDanger        = "#F87171"
	runRequestPickerColMethodGet     = "#34d399"
	runRequestPickerColMethodPost    = "#60a5fa"
	runRequestPickerColMethodPut     = "#f59e0b"
	runRequestPickerColMethodPatch   = "#14b8a6"
	runRequestPickerColMethodHead    = "#a1a1aa"
	runRequestPickerColMethodOptions = "#c084fc"
	runRequestPickerColMethodGRPC    = "#22d3ee"
	runRequestPickerColMethodWS      = "#fb923c"
	runRequestPickerColMethodDefault = "#9ca3af"
)

var runRequestPickerMethodCols = map[string]string{
	"GET":     runRequestPickerColMethodGet,
	"POST":    runRequestPickerColMethodPost,
	"PUT":     runRequestPickerColMethodPut,
	"PATCH":   runRequestPickerColMethodPatch,
	"DELETE":  runRequestPickerColDanger,
	"HEAD":    runRequestPickerColMethodHead,
	"OPTIONS": runRequestPickerColMethodOptions,
	"GRPC":    runRequestPickerColMethodGRPC,
	"WS":      runRequestPickerColMethodWS,
	"":        runRequestPickerColMethodDefault,
}

func promptRunRequestChoiceTTY(
	r io.Reader,
	w io.Writer,
	path string,
	choices []RunRequestChoice,
	opt RunRequestPromptOptions,
) (RunRequestChoice, error) {
	in, inOK := r.(*os.File)
	out, outOK := w.(*os.File)
	if !inOK || !outOK || !term.IsTerminal(int(in.Fd())) || !term.IsTerminal(int(out.Fd())) {
		return promptRunRequestChoiceText(r, w, path, choices)
	}
	m := newRunRequestPickerModel(path, choices, opt.Color)
	if wid, hgt, ok := runRequestPromptSize(out); ok {
		m.setSize(wid, hgt)
	}

	p := tea.NewProgram(
		m,
		tea.WithInput(r),
		tea.WithOutput(w),
		tea.WithoutSignalHandler(),
	)
	got, err := p.Run()
	if err != nil {
		if errors.Is(err, tea.ErrInterrupted) {
			return RunRequestChoice{}, ErrRunRequestChoiceCanceled
		}
		return RunRequestChoice{}, err
	}
	pm, ok := got.(*runRequestPickerModel)
	if !ok || pm == nil {
		return RunRequestChoice{}, errors.New("request selection failed")
	}
	if pm.canceled {
		return RunRequestChoice{}, ErrRunRequestChoiceCanceled
	}
	if pm.done {
		return pm.choice, nil
	}
	return RunRequestChoice{}, io.EOF
}

func runRequestPromptSize(f *os.File) (int, int, bool) {
	if f == nil {
		return 0, 0, false
	}
	fd := int(f.Fd())
	if !term.IsTerminal(fd) {
		return 0, 0, false
	}
	wid, hgt, err := term.GetSize(fd)
	if err != nil {
		return 0, 0, false
	}
	return wid, hgt, true
}

type runRequestPickerModel struct {
	path     string
	choices  []RunRequestChoice
	sel      int
	top      int
	wid      int
	rows     int
	num      string
	note     string
	done     bool
	canceled bool
	choice   RunRequestChoice
	st       runRequestPickerStyle
}

func newRunRequestPickerModel(
	path string,
	choices []RunRequestChoice,
	cfg termcolor.Config,
) *runRequestPickerModel {
	m := &runRequestPickerModel{
		path:    path,
		choices: choices,
		wid:     runRequestPickerDefaultWidth,
		rows:    runRequestPickerDefaultRows,
		st:      newRunRequestPickerStyle(cfg),
	}
	m.keepVisible()
	return m
}

func (m *runRequestPickerModel) Init() tea.Cmd {
	return nil
}

func (m *runRequestPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		switch key := msg.String(); key {
		case "ctrl+c", "esc", "q":
			m.canceled = true
			return m, tea.Quit
		case "up", "k", "ctrl+p":
			m.move(-1)
		case "down", "j", "ctrl+n":
			m.move(1)
		case "pgup", "ctrl+b":
			m.page(-1)
		case "pgdown", "ctrl+f":
			m.page(1)
		case "home":
			m.selectFirst()
		case "end":
			m.selectLast()
		case "backspace", "ctrl+h":
			m.backspace()
		case "enter":
			ch, ok := m.confirm()
			if ok {
				m.choice = ch
				m.done = true
				return m, tea.Quit
			}
		default:
			if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
				m.appendDigit(key[0])
			}
		}
		if m.done || m.canceled {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *runRequestPickerModel) View() string {
	if m == nil {
		return ""
	}
	box := m.st.box
	inner := m.wid - box.GetHorizontalFrameSize()
	if inner < 1 {
		inner = 1
	}
	lines := []string{
		m.st.title.Render("Select a Request to Run"),
		m.st.path.Render(xansi.TruncateLeft(m.path, inner, "...")),
		m.st.meta.Render(m.rangeText()),
		"",
	}
	for i := range m.visibleChoices() {
		idx := m.top + i
		lines = append(lines, m.renderRow(inner, idx, m.choices[idx], idx == m.sel))
	}
	lines = append(lines, "")
	lines = append(lines, m.detailLine(inner))
	lines = append(lines, m.helpLine(inner))
	return box.Render(strings.Join(lines, "\n")) + "\n"
}

func (m *runRequestPickerModel) setSize(wid, hgt int) {
	if wid > 0 {
		wid -= runRequestPickerSideMargin
		if wid <= 0 {
			wid = 1
		}
		if wid > runRequestPickerMaxWidth {
			wid = runRequestPickerMaxWidth
		}
		m.wid = wid
	}
	if hgt > 0 {
		rows := hgt - runRequestPickerChromeRows
		if rows < runRequestPickerMinRows {
			rows = runRequestPickerMinRows
		}
		if rows > runRequestPickerMaxRows {
			rows = runRequestPickerMaxRows
		}
		m.rows = rows
	}
	m.keepVisible()
}

func (m *runRequestPickerModel) move(delta int) {
	if len(m.choices) == 0 {
		return
	}
	m.clearInput()
	m.sel += delta
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.choices) {
		m.sel = len(m.choices) - 1
	}
	m.keepVisible()
}

func (m *runRequestPickerModel) page(delta int) {
	step := max(m.rows-1, 1)
	m.move(delta * step)
}

func (m *runRequestPickerModel) selectFirst() {
	m.clearInput()
	m.sel = 0
	m.keepVisible()
}

func (m *runRequestPickerModel) selectLast() {
	if len(m.choices) == 0 {
		return
	}
	m.clearInput()
	m.sel = len(m.choices) - 1
	m.keepVisible()
}

func (m *runRequestPickerModel) clearInput() {
	m.num = ""
	m.note = ""
}

func (m *runRequestPickerModel) appendDigit(ch byte) {
	if ch < '0' || ch > '9' {
		return
	}
	next := m.num + string(ch)
	maxDigits := len(strconv.Itoa(len(m.choices)))
	if maxDigits < 1 {
		maxDigits = 1
	}
	if len(next) > maxDigits || !m.validNumberInput(next) {
		next = string(ch)
	}
	m.num = next
	m.applyNumber()
}

func (m *runRequestPickerModel) backspace() {
	if m.num == "" {
		return
	}
	m.num = m.num[:len(m.num)-1]
	m.applyNumber()
}

func (m *runRequestPickerModel) applyNumber() {
	m.note = ""
	if m.num == "" {
		return
	}
	idx, ok := m.numberIndex()
	if !ok {
		m.note = fmt.Sprintf("Pick a request number from 1 to %d.", len(m.choices))
		return
	}
	m.sel = idx
	m.keepVisible()
}

func (m *runRequestPickerModel) numberIndex() (int, bool) {
	n, err := strconv.Atoi(m.num)
	if err != nil || n < 1 || n > len(m.choices) {
		return 0, false
	}
	return n - 1, true
}

func (m *runRequestPickerModel) validNumberInput(raw string) bool {
	n, err := strconv.Atoi(raw)
	return err == nil && n >= 1 && n <= len(m.choices)
}

func (m *runRequestPickerModel) confirm() (RunRequestChoice, bool) {
	if len(m.choices) == 0 {
		return RunRequestChoice{}, false
	}
	if m.num != "" {
		idx, ok := m.numberIndex()
		if !ok {
			m.note = fmt.Sprintf("Pick a request number from 1 to %d.", len(m.choices))
			return RunRequestChoice{}, false
		}
		m.sel = idx
	}
	return m.choices[m.sel], true
}

func (m *runRequestPickerModel) keepVisible() {
	if len(m.choices) == 0 {
		m.sel = 0
		m.top = 0
		return
	}
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.choices) {
		m.sel = len(m.choices) - 1
	}
	if m.rows <= 0 {
		m.rows = runRequestPickerDefaultRows
	}
	if m.sel < m.top {
		m.top = m.sel
	}
	if m.sel >= m.top+m.rows {
		m.top = m.sel - m.rows + 1
	}
	maxTop := max(len(m.choices)-m.rows, 0)
	if m.top > maxTop {
		m.top = maxTop
	}
	if m.top < 0 {
		m.top = 0
	}
}

func (m *runRequestPickerModel) visibleChoices() []RunRequestChoice {
	if len(m.choices) == 0 {
		return nil
	}
	end := min(m.top+m.rows, len(m.choices))
	return m.choices[m.top:end]
}

func (m *runRequestPickerModel) rangeText() string {
	if len(m.choices) == 0 {
		return "No requests found."
	}
	start := m.top + 1
	end := min(m.top+m.rows, len(m.choices))
	if len(m.choices) <= m.rows {
		return fmt.Sprintf("%d requests found", len(m.choices))
	}
	return fmt.Sprintf("%d requests found. Showing %d-%d.", len(m.choices), start, end)
}

func (m *runRequestPickerModel) renderRow(
	wid int,
	idx int,
	ch RunRequestChoice,
	sel bool,
) string {
	rowSt := m.st.row
	curSt := m.st.cursor
	numSt := m.st.number
	nameSt := m.st.name
	lineSt := m.st.line
	if sel {
		rowSt = m.st.rowSel
		curSt = m.st.cursorSel
		numSt = m.st.numberSel
		nameSt = m.st.nameSel
		lineSt = m.st.lineSel
	}

	mark := curSt.Render(" ")
	if sel {
		mark = curSt.Render(">")
	}
	num := numSt.Render(fmt.Sprintf("%*d.", len(strconv.Itoa(len(m.choices))), idx+1))
	mth := m.st.method(ch.Method)
	name := ch.Name
	if name == "" {
		name = ch.Label
	}
	badge := ""
	if ch.Line > 0 {
		badge = lineSt.Render(fmt.Sprintf("L%d", ch.Line))
	}

	head := mark + " " + num
	if mth != "" {
		head += " " + mth
	}

	compact := wid < runRequestPickerCompactWidth
	reserve := 0
	if badge != "" {
		if compact {
			reserve = lipgloss.Width(" " + badge)
		} else {
			reserve = lipgloss.Width(badge) + 1
		}
	}
	avail := wid - lipgloss.Width(head) - 1 - reserve
	if avail < 1 {
		avail = 1
	}
	body := head + " " + nameSt.Render(xansi.Truncate(name, avail, "..."))
	if badge != "" {
		if compact {
			body += " " + badge
		} else {
			pad := wid - lipgloss.Width(body) - lipgloss.Width(badge)
			if pad < 1 {
				pad = 1
			}
			body += strings.Repeat(" ", pad) + badge
		}
	}
	return rowSt.Width(wid).Render(body)
}

func (m *runRequestPickerModel) detailLine(wid int) string {
	if m.note != "" {
		return m.st.note.Render(xansi.Truncate(m.note, wid, "..."))
	}
	if len(m.choices) == 0 || m.sel < 0 || m.sel >= len(m.choices) {
		return ""
	}
	ch := m.choices[m.sel]
	var parts []string
	rem := wid
	if m.num != "" {
		text := fmt.Sprintf("Jump: %s", m.num)
		parts = append(parts, m.st.numberSel.Render(text))
		rem -= xansi.StringWidth(text) + 2
	}
	if ch.Line > 0 {
		text := fmt.Sprintf("line %d", ch.Line)
		parts = append(parts, m.st.line.Render(text))
		rem -= xansi.StringWidth(text) + 2
	}
	if tgt := strings.TrimSpace(ch.Target); tgt != "" && tgt != ch.Name {
		if rem < 1 {
			rem = 1
		}
		parts = append(parts, m.st.target.Render(xansi.Truncate(tgt, rem, "...")))
	}
	return strings.Join(parts, "  ")
}

func (m *runRequestPickerModel) helpLine(wid int) string {
	msg := "Use up/down to move, type a number to jump, Enter to run."
	return m.st.help.Render(xansi.Truncate(msg, wid, "..."))
}

type runRequestPickerStyle struct {
	box       lipgloss.Style
	title     lipgloss.Style
	path      lipgloss.Style
	meta      lipgloss.Style
	row       lipgloss.Style
	rowSel    lipgloss.Style
	cursor    lipgloss.Style
	cursorSel lipgloss.Style
	number    lipgloss.Style
	numberSel lipgloss.Style
	name      lipgloss.Style
	nameSel   lipgloss.Style
	line      lipgloss.Style
	lineSel   lipgloss.Style
	target    lipgloss.Style
	help      lipgloss.Style
	note      lipgloss.Style
	methods   map[string]lipgloss.Style
}

func newRunRequestPickerStyle(cfg termcolor.Config) runRequestPickerStyle {
	r := lipgloss.NewRenderer(io.Discard)
	if cfg.Enabled {
		r.SetColorProfile(cfg.Profile)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}
	st := runRequestPickerStyle{
		box: r.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(runRequestPickerColBorder)).
			Padding(0, 1),
		title: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColTitle)).
			Bold(true),
		path: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColPath)),
		meta: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColMuted)),
		row: r.NewStyle(),
		rowSel: r.NewStyle().
			Background(lipgloss.Color(runRequestPickerColSelBg)).
			Foreground(lipgloss.Color(runRequestPickerColSelFg)).
			Bold(true),
		cursor: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColCursor)),
		cursorSel: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColAccent)).
			Bold(true),
		number: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColMuted)),
		numberSel: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColTitle)).
			Bold(true),
		name: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColText)),
		nameSel: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColBright)).
			Bold(true),
		line: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColMuted)),
		lineSel: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColLineSel)),
		target: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColTarget)),
		help: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColMuted)),
		note: r.NewStyle().
			Foreground(lipgloss.Color(runRequestPickerColDanger)).
			Bold(true),
		methods: make(map[string]lipgloss.Style),
	}
	for k, v := range runRequestPickerMethodCols {
		st.methods[k] = r.NewStyle().
			Foreground(lipgloss.Color(v)).
			Bold(true)
	}
	return st
}

func (s runRequestPickerStyle) method(m string) string {
	m = str.UpperTrim(m)
	if m == "" {
		return ""
	}
	st, ok := s.methods[m]
	if !ok {
		st = s.methods[""]
	}
	return st.Render(m)
}
