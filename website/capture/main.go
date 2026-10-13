package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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
	ID   string   `json:"id"`
	Keys []string `json:"keys"`
	Wait string   `json:"wait"`
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

type frames struct {
	Cols   int     `json:"cols"`
	Rows   int     `json:"rows"`
	Styles []style `json:"styles"`
	Frames []frame `json:"frames"`
}

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
	name string
	def  theme.Definition
	dark bool
	pre  []string
}

func main() {
	dir := flag.String("dir", "website/src/content/tour", "tour directory")
	flag.Parse()
	// Keep captured timestamps independent of the local timezone.
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
		hidden := v
		hidden.name += "-editor-hidden"
		hidden.pre = []string{"g", "2"}
		vs = append(vs, v, hidden)
	}
	for _, v := range vs {
		fs, err := capture(t, *dir, version, cat, v)
		if err != nil {
			log.Fatalf("%s: %v", v.name, err)
		}
		js, err := json.Marshal(fs)
		if err != nil {
			log.Fatal(err)
		}
		out := filepath.Join(*dir, "frames-"+v.name+".json")
		if err := os.WriteFile(out, js, 0o644); err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s (%d frames, %d bytes)", out, len(fs.Frames), len(js))
	}
}

func capture(t tour, dir, version string, cat theme.Catalog, v variant) (frames, error) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(v.dark)

	tmp, err := os.MkdirTemp("", "resterm-capture")
	if err != nil {
		return frames{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// Isolate theme and layout saves from the user's settings.
	if err := os.Setenv("RESTERM_CONFIG_DIR", filepath.Join(tmp, "config")); err != nil {
		return frames{}, err
	}
	ws := filepath.Join(tmp, "demo-api")
	if err := os.Mkdir(ws, 0o755); err != nil {
		return frames{}, err
	}
	for _, name := range []string{t.File, "resterm.env.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return frames{}, err
		}
		if err := os.WriteFile(filepath.Join(ws, name), b, 0o644); err != nil {
			return frames{}, err
		}
	}

	path := filepath.Join(ws, t.File)
	flags := cli.NewExecFlags()
	flags.EnvName = t.Env
	sess, err := flags.Resolve(path)
	if err != nil {
		return frames{}, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return frames{}, err
	}

	m := ui.New(ui.Config{
		// At 160 columns the file badges, editor lines and response tabs all stay whole.
		Settings:       config.Settings{Layout: config.LayoutSettings{SidebarWidth: 0.23, EditorSplit: 0.5}},
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
			// Exit rather than leave snap waiting on a stopped program.
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
	// Git status rebuilds the navigator and can move the selection.
	time.Sleep(time.Second)
	snap := func() string {
		out := make(chan string, 1)
		p.Send(snapMsg{out})
		return <-out
	}
	for _, k := range v.pre {
		p.Send(key(k))
		time.Sleep(60 * time.Millisecond)
	}

	fs := frames{Cols: t.Cols, Rows: t.Rows}
	ids := map[style]int{}
	for _, st := range t.Steps {
		// Wait for a new match; the editor may already contain the text.
		seen := strings.Count(ansi.Strip(snap()), st.Wait)
		for _, k := range st.Keys {
			p.Send(key(k))
			time.Sleep(60 * time.Millisecond)
		}
		deadline := time.Now().Add(10 * time.Second)
		view := snap()
		for strings.Count(ansi.Strip(view), st.Wait) <= seen {
			if time.Now().After(deadline) {
				return frames{}, fmt.Errorf("step %s: timed out waiting for %q\n%s", st.ID, st.Wait, ansi.Strip(view))
			}
			time.Sleep(50 * time.Millisecond)
			view = snap()
		}
		// Let animations settle.
		time.Sleep(400 * time.Millisecond)
		view = snap()
		fs.Frames = append(fs.Frames, frame{ID: st.ID, Lines: grid(view, t.Cols, t.Rows, ids, &fs.Styles)})
	}
	return fs, nil
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// grid groups cells into style runs. Style IDs are shared across frames.
func grid(view string, w, h int, ids map[style]int, styles *[]style) [][]any {
	buf := cellbuf.NewBuffer(w, h)
	cellbuf.SetContent(buf, view)
	lines := make([][]any, 0, h)
	for y := range h {
		var line []any
		var text strings.Builder
		cur := -1
		flush := func() {
			if text.Len() > 0 {
				line = append(line, []any{cur, text.String()})
				text.Reset()
			}
		}
		for x := range w {
			c := buf.Cell(x, y)
			if c.Width == 0 {
				continue
			}
			st := toStyle(c.Style)
			id, ok := ids[st]
			if !ok {
				id = len(*styles)
				ids[st] = id
				*styles = append(*styles, st)
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
