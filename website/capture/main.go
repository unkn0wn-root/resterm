package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/config"
	"github.com/unkn0wn-root/resterm/internal/theme"
	"github.com/unkn0wn-root/resterm/internal/ui"
)

type tour struct {
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
	File  string `json:"file"`
	Env   string `json:"env"`
	Steps []step `json:"steps"`
}

type step struct {
	ID    string   `json:"id"`
	Keys  []string `json:"keys"`
	Wait  string   `json:"wait"`
	Focus string   `json:"focus"`
	Split float64  `json:"split"`
	Panes []string `json:"panes"`
}

type style struct {
	Fg string `json:"fg,omitempty"`
	Bg string `json:"bg,omitempty"`
	A  string `json:"a,omitempty"`
}

type frame struct {
	ID    string  `json:"id"`
	Lines [][]any `json:"lines"`
}

type table struct {
	Styles []style `json:"styles"`
	ids    map[style]int
}

type frames struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
	table
	Frames []frame `json:"frames"`
}

type pane struct {
	ID    string  `json:"id"`
	Box   [4]int  `json:"box"`
	Focus [][]any `json:"focus,omitempty"`
	Body  *body   `json:"body,omitempty"`
}

type body struct {
	X     int  `json:"x"`
	Y     int  `json:"y"`
	W     int  `json:"w"`
	H     int  `json:"h"`
	Strip int  `json:"strip"`
	Keep  bool `json:"keep,omitempty"` // Whether scroll position survives a focus change.
}

type paneFrame struct {
	ID    string `json:"id"`
	Panes []pane `json:"panes"`
}

type paneData struct {
	table
	Strips [][][]any   `json:"strips"`
	Frames []paneFrame `json:"frames"`
	seen   map[string]int
}

func (d *paneData) strip(lines [][]any) int {
	if d.seen == nil {
		d.seen = map[string]int{}
	}
	k := enc(lines)
	i, ok := d.seen[k]
	if !ok {
		i = len(d.Strips)
		d.seen[k] = i
		d.Strips = append(d.Strips, lines)
	}
	return i
}

var (
	titles    = map[string]string{"requests": "Files", "editor": "Editor", "response": "Response"}
	focusKeys = map[string][]string{"requests": {"g", "r"}, "editor": {"g", "i"}, "response": {"g", "p"}}
)

const (
	// Resterm scrolls up to three rows per wheel notch.
	notch     = 3
	tries     = 5
	settle    = 400 * time.Millisecond
	splitStep = 0.05
)

type snapMsg struct{ out chan<- string }

type shooter struct{ tea.Model }

func (s shooter) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(snapMsg); ok {
		m.out <- s.View()
		return s, nil
	}
	next, cmd := s.Model.Update(msg)
	s.Model = next
	return s, cmd
}

type variant struct {
	name   string
	def    theme.Definition
	dark   bool
	hidden bool
}

func main() {
	dir := flag.String("dir", "website/src/content/tour", "tour directory")
	flag.Parse()
	// Use UTC so captures don't depend on the machine's timezone.
	time.Local = time.UTC

	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		log.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^#{2,3} \[(\d+\.\d+\.\d+)\]`).FindSubmatch(changelog)
	if m == nil {
		log.Fatal("no release heading in CHANGELOG.md")
	}
	version := string(m[1])

	var t tour
	data, err := os.ReadFile(filepath.Join(*dir, "tour.json"))
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &t); err != nil {
		log.Fatalf("tour.json: %v", err)
	}

	cat, err := theme.LoadCatalog([]string{"_examples/themes"})
	if err != nil {
		log.Fatal(err)
	}
	light, ok := cat.Get("daybreak")
	if !ok {
		log.Fatal("daybreak theme not found in _examples/themes")
	}

	var vs []variant
	for _, v := range []variant{{name: "dark", def: theme.DefaultDefinition(), dark: true}, {name: "light", def: light}} {
		h := v
		h.name += "-editor-hidden"
		h.hidden = true
		vs = append(vs, v, h)
	}
	for _, v := range vs {
		fs, pd, err := capture(t, *dir, version, cat, v)
		if err != nil {
			log.Fatalf("%s: %v", v.name, err)
		}
		write(filepath.Join(*dir, "frames-"+v.name+".json"), fs)
		write(filepath.Join(*dir, "panes-"+v.name+".json"), pd)
	}
}

func write(path string, v any) {
	js, err := json.Marshal(v)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, js, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(js))
}

func capture(t tour, dir, version string, cat theme.Catalog, v variant) (frames, paneData, error) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(v.dark)

	tmp, err := os.MkdirTemp("", "resterm-capture")
	if err != nil {
		return frames{}, paneData{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// Don't save the capture's theme and layout to the user's settings.
	if err := os.Setenv("RESTERM_CONFIG_DIR", filepath.Join(tmp, "config")); err != nil {
		return frames{}, paneData{}, err
	}
	ws := filepath.Join(tmp, "demo-api")
	if err := os.Mkdir(ws, 0o755); err != nil {
		return frames{}, paneData{}, err
	}
	for _, name := range []string{t.File, "resterm.env.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return frames{}, paneData{}, err
		}
		if err := os.WriteFile(filepath.Join(ws, name), b, 0o644); err != nil {
			return frames{}, paneData{}, err
		}
	}

	path := filepath.Join(ws, t.File)
	flags := cli.NewExecFlags()
	flags.EnvName = t.Env
	sess, err := flags.Resolve(path)
	if err != nil {
		return frames{}, paneData{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return frames{}, paneData{}, err
	}

	m := ui.New(ui.Config{
		// The sidebar width fits file badges at 160 columns.
		Settings: config.Settings{
			Layout: config.LayoutSettings{SidebarWidth: 0.23, EditorSplit: config.LayoutEditorSplitDefault},
		},
		FilePath:       path,
		InitialContent: string(content),
		Theme:          &v.def.Theme,
		ThemeCatalog:   cat,
		ActiveThemeKey: v.def.Key,
		Env:            sess.Env,
		HTTPOptions:    sess.HTTPOpts,
		GRPCOptions:    sess.GRPCOpts,
		WorkspaceRoot:  sess.Workspace,
		Version:        version,
		StatusUser:     "you",
		StatusHost:     "devbox",
	})
	p := tea.NewProgram(shooter{m}, tea.WithInput(nil), tea.WithOutput(io.Discard))
	done := make(chan tea.Model, 1)
	go func() {
		final, err := p.Run()
		if err != nil {
			// A stopped program can't answer snapshots, so continuing would hang.
			log.Fatal(err)
		}
		done <- final
	}()
	defer func() {
		p.Quit()
		final := (<-done).(shooter).Model.(ui.Model)
		if err := final.Close(); err != nil {
			log.Print(err)
		}
	}()

	p.Send(tea.WindowSizeMsg{Width: t.Cols, Height: t.Rows})
	// Wait for the initial Git status update, which can move the selection.
	time.Sleep(time.Second)
	s := session{p: p, cols: t.Cols, rows: t.Rows}
	if v.hidden {
		s.press("g", "2")
	}

	split := config.LayoutEditorSplitDefault
	fs := frames{Cols: t.Cols, Rows: t.Rows}
	var pd paneData
	for _, st := range t.Steps {
		// Count existing matches; the editor may already show the wait text.
		seen := strings.Count(ansi.Strip(s.snap()), st.Wait)
		s.press(st.Keys...)
		deadline := time.Now().Add(10 * time.Second)
		view := s.snap()
		for strings.Count(ansi.Strip(view), st.Wait) <= seen {
			if time.Now().After(deadline) {
				err := fmt.Errorf("step %s: timed out waiting for %q\n%s", st.ID, st.Wait, ansi.Strip(view))
				return frames{}, paneData{}, err
			}
			time.Sleep(50 * time.Millisecond)
			view = s.snap()
		}
		time.Sleep(settle)
		if st.Split != 0 && !v.hidden {
			k := "l"
			if st.Split < split {
				k = "h"
			}
			s.press(focusKeys["response"]...)
			for range int(math.Round(math.Abs(st.Split-split) / splitStep)) {
				s.press("g", k)
			}
			s.press(focusKeys[st.Focus]...)
			split = st.Split
			time.Sleep(settle)
		}
		view = s.snap()
		buf := s.screen(view)
		fs.Frames = append(fs.Frames, frame{ID: st.ID, Lines: fs.lines(buf, 0, t.Cols, 0, t.Rows)})

		pf := paneFrame{ID: st.ID, Panes: []pane{}}
		for _, id := range st.Panes {
			r, ok := find(buf, titles[id])
			if !ok {
				continue
			}
			pn := pane{ID: id, Box: [4]int{r.x0, r.y0, r.x1 - r.x0 + 1, r.y1 - r.y0 + 1}}
			if id != st.Focus {
				s.press(focusKeys[id]...)
				time.Sleep(settle)
				pn.Focus = pd.lines(s.screen(s.snap()), 0, t.Cols, 0, t.Rows)
			}
			// Wheel events change the selected request.
			if id != "requests" {
				b, err := s.scroll(view, r, id, st.Focus, &pd)
				if err != nil {
					return frames{}, paneData{}, fmt.Errorf("step %s: %s: %w", st.ID, id, err)
				}
				pn.Body = b
			}
			pf.Panes = append(pf.Panes, pn)

			// The next step's keys assume the original focus and scroll position.
			s.press(focusKeys[st.Focus]...)
			time.Sleep(settle)
			if got := s.snap(); got != view {
				var diff strings.Builder
				g, w := strings.Split(got, "\n"), strings.Split(view, "\n")
				for i := range min(len(g), len(w)) {
					if g[i] != w[i] {
						fmt.Fprintf(&diff, "row %d\n  want %s\n  got  %s\n", i, ansi.Strip(w[i]), ansi.Strip(g[i]))
					}
				}
				return frames{}, paneData{}, fmt.Errorf(
					"step %s: %s: the screen did not come back\n%s",
					st.ID,
					id,
					diff.String(),
				)
			}
		}
		pd.Frames = append(pd.Frames, pf)
	}
	return fs, pd, nil
}

type session struct {
	p          *tea.Program
	cols, rows int
}

func (s session) snap() string {
	out := make(chan string, 1)
	s.p.Send(snapMsg{out})
	return <-out
}

func (s session) press(keys ...string) {
	for _, k := range keys {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		}
		s.p.Send(msg)
		time.Sleep(60 * time.Millisecond)
	}
}

func (s session) wheel(x, y, n int) {
	b := tea.MouseButtonWheelDown
	if n < 0 {
		b, n = tea.MouseButtonWheelUp, -n
	}
	for range n {
		s.p.Send(tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress})
	}
}

func (s session) screen(view string) *cellbuf.Buffer {
	buf := cellbuf.NewBuffer(s.cols, s.rows)
	cellbuf.SetContent(buf, view)
	return buf
}

// scroll captures a pane's full content, or returns nil if it fits on screen.
// The caller must restore focus afterward.
func (s session) scroll(base string, r rect, id, focus string, pd *paneData) (*body, error) {
	x, y := (r.x0+r.x1)/2, (r.y0+r.y1)/2
	rows := func(view string, top int) [][]any { return pd.lines(s.screen(view), r.x0+1, r.x1, top, r.y1) }

	// Editor ticks can reset scrolling to the cursor. Retry snapshots from the top.
	var down, on string
	for range tries {
		s.wheel(x, y, 1)
		down = s.snap()
		s.wheel(x, y, -1)
		on = s.snap()
		if down != on {
			break
		}
	}
	// Skip fixed rows, such as response tabs, above the scrolling content.
	a, b := rows(on, r.y0+1), rows(down, r.y0+1)
	i := 0
	for i < len(a) && enc(a[i]) == enc(b[i]) {
		i++
	}
	if i == len(a) {
		return nil, nil
	}
	top, ons := r.y0+1+i, a[i:]
	h := len(ons)

	// Focus the response pane to avoid moving the editor to the selected request.
	other := "requests"
	if id == "editor" {
		other = "response"
	}
	idle := func() [][]any {
		s.press(focusKeys[other]...)
		return rows(s.snap(), top)
	}
	off := idle()
	want := off
	if id == focus {
		want = ons
	}
	if enc(want) != enc(rows(base, top)) {
		return nil, errors.New("the body at the top differs from the step screen")
	}
	s.wheel(x, y, 1)
	keep := enc(idle()) != enc(off)
	if keep && enc(off) != enc(ons) {
		return nil, errors.New("the idle body differs from the focused one")
	}

	at := func(k int) [][]any {
		s.wheel(x, y, -k)
		s.wheel(x, y, k)
		return rows(s.snap(), top)
	}
	prev, k := ons, 1
next:
	for ; ; k++ {
		var cur [][]any
		n := 0
		for range tries {
			cur = at(k)
			if enc(cur) == enc(prev) {
				// Read again to distinguish the end of the pane from an editor scroll reset.
				if enc(at(k)) == enc(prev) {
					break next
				}
				continue
			}
			n = notch
			for n > 0 && enc(cur[:h-n]) != enc(prev[n:]) {
				n--
			}
			if n > 0 {
				break
			}
		}
		if n == 0 {
			return nil, fmt.Errorf("wheel notch %d did not shift the body by up to %d rows", k, notch)
		}
		if keep && enc(idle()) != enc(cur) {
			return nil, fmt.Errorf("wheel notch %d: the idle body differs from the focused one", k)
		}
		ons = append(ons, cur[h-n:]...)
		prev = cur
	}
	s.wheel(x, y, -k)

	// The editor can scroll past its last line. Trim the trailing blank rows.
	for id == "editor" && len(ons) > h {
		var text strings.Builder
		for _, run := range ons[len(ons)-1] {
			text.WriteString(run.([]any)[1].(string))
		}
		if strings.Trim(text.String(), " ┃") != "" {
			break
		}
		ons = ons[:len(ons)-1]
	}
	return &body{X: r.x0 + 1, Y: top, W: r.x1 - r.x0 - 1, H: h, Strip: pd.strip(ons), Keep: keep}, nil
}

// rect includes the pane's border cells; x1 and y1 are inclusive.
type rect struct{ x0, y0, x1, y1 int }

func find(buf *cellbuf.Buffer, title string) (rect, bool) {
	w, h := buf.Width(), buf.Height()
	for y := range h {
		row := make([]rune, w)
		for x := range w {
			row[x] = buf.Cell(x, y).Rune
		}
		i := strings.Index(string(row), "─ "+title+" ─")
		if i < 0 {
			continue
		}
		r := rect{x0: utf8.RuneCountInString(string(row)[:i]), y0: y}
		r.x1 = r.x0
		for r.x0 >= 0 && row[r.x0] != '╭' {
			r.x0--
		}
		for r.x1 < w && row[r.x1] != '╮' {
			r.x1++
		}
		if r.x0 < 0 || r.x1 == w {
			return rect{}, false
		}
		r.y1 = y + 1
		for r.y1 < h && buf.Cell(r.x0, r.y1).Rune != '╰' {
			r.y1++
		}
		return r, r.y1 < h
	}
	return rect{}, false
}

func enc(v any) string {
	js, _ := json.Marshal(v)
	return string(js)
}

func (t *table) lines(buf *cellbuf.Buffer, x0, x1, y0, y1 int) [][]any {
	if t.ids == nil {
		t.ids = map[style]int{}
	}
	lines := make([][]any, 0, y1-y0)
	for y := y0; y < y1; y++ {
		var line []any
		var text strings.Builder
		cur := -1
		flush := func() {
			if text.Len() > 0 {
				line = append(line, []any{cur, text.String()})
				text.Reset()
			}
		}
		for x := x0; x < x1; x++ {
			c := buf.Cell(x, y)
			if c.Width == 0 {
				continue
			}
			st := toStyle(c.Style)
			id, ok := t.ids[st]
			if !ok {
				id = len(t.Styles)
				t.ids[st] = id
				t.Styles = append(t.Styles, st)
			}
			r := c.String()
			if c.Width > 1 {
				flush()
				line = append(line, []any{id, r, c.Width})
				cur = -1
				continue
			}
			if id != cur {
				flush()
				cur = id
			}
			text.WriteString(r)
		}
		flush()
		lines = append(lines, line)
	}
	return lines
}

func toStyle(s cellbuf.Style) style {
	var a strings.Builder
	for _, f := range []struct {
		m cellbuf.AttrMask
		c byte
	}{{cellbuf.BoldAttr, 'b'}, {cellbuf.FaintAttr, 'f'}, {cellbuf.ItalicAttr, 'i'}, {cellbuf.ReverseAttr, 'r'}} {
		if s.Attrs&f.m != 0 {
			a.WriteByte(f.c)
		}
	}
	if s.UlStyle != cellbuf.NoUnderline {
		a.WriteByte('u')
	}
	return style{Fg: hex(s.Fg), Bg: hex(s.Bg), A: a.String()}
}

func hex(c color.Color) string {
	switch c.(type) {
	case nil:
		return ""
	case ansi.BasicColor, ansi.IndexedColor:
		return fmt.Sprintf("@%d", c)
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}
