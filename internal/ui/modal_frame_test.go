package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/prompt"
	"github.com/unkn0wn-root/resterm/internal/theme"
)

// Disable time-based rendering so repeated frames are deterministic.
func steadyFrame(m *Model) {
	m.latAnimOn = false
}

func TestCachedModalRenderMatchesUncached(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	tests := []struct {
		name   string
		width  int
		height int
		box    func(*Model) string
	}{
		{
			name:   "ANSI colors",
			width:  80,
			height: 24,
			box: func(m *Model) string {
				body := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ffcc66")).
					Background(lipgloss.Color("#112233")).
					Render("colored modal")
				return m.theme.BrowserBorder.Width(32).Render(body)
			},
		},
		{
			name:   "wide Unicode",
			width:  53,
			height: 15,
			box: func(m *Model) string {
				return m.theme.BrowserBorder.Width(21).Render("界面 🚀\nnaïve é")
			},
		},
		{
			name:   "small terminal and uneven lines",
			width:  9,
			height: 4,
			box: func(*Model) string {
				return "\x1b[31m1234567890\x1b[0m\n界"
			},
		},
		{
			name:   "box wider and taller than the screen",
			width:  10,
			height: 3,
			box: func(m *Model) string {
				return m.theme.BrowserBorder.Width(40).Render("overflowing\nmodal\nbody\ntext")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelWithDoc(sampleRequestDoc)
			m.width = tt.width
			m.height = tt.height
			m.ready = true
			steadyFrame(m)
			m.theme.ModalBackdrop = lipgloss.Color("#0f1720")
			_ = m.applyLayout()
			box := tt.box(m)

			m.invalidateModalRender()
			want := m.renderCenteredModal(box)
			cached := m.renderCenteredModal(box)
			if cached != want {
				t.Fatalf("cached frame differs\ncached: %q\nwant:   %q", cached, want)
			}

			m.invalidateModalRender()
			if rebuilt := m.renderCenteredModal(box); rebuilt != want {
				t.Fatalf("rebuilt frame differs\nrebuilt: %q\nwant:    %q", rebuilt, want)
			}
		})
	}
}

func TestModalDrawDoesNotDisturbCachedUnderlay(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	m := newTestModelWithDoc(sampleRequestDoc)
	m.width = 60
	m.height = 20
	m.ready = true
	steadyFrame(m)
	m.theme.ModalBackdrop = lipgloss.Color("#0f1720")
	_ = m.applyLayout()

	first := m.theme.BrowserBorder.Width(30).Render("界面 first\nsecond line")
	second := m.theme.BrowserBorder.Width(30).Render("plain second\nother line!")
	if lipgloss.Width(first) != lipgloss.Width(second) ||
		lipgloss.Height(first) != lipgloss.Height(second) {
		t.Fatal("test boxes must share geometry so they share a cache entry")
	}

	m.invalidateModalRender()
	_ = m.renderCenteredModal(first)
	got := m.renderCenteredModal(second)

	m.invalidateModalRender()
	want := m.renderCenteredModal(second)
	if got != want {
		t.Fatalf("second modal reused a dirtied underlay\ngot:  %q\nwant: %q", got, want)
	}
}

func TestPromptModalCachePreservesErrorsAndSuggestions(t *testing.T) {
	m := newTestModelWithDoc(sampleRequestDoc)
	m.width = 100
	m.height = 32
	m.ready = true
	steadyFrame(m)
	m.openPathPrompt.menu.Reset([]prompt.Item{
		{Label: "api/", Summary: "directory"},
		{Label: "界.http", Summary: "workspace file"},
	})
	m.showOpenModal = true
	_ = m.applyLayout()

	first := m.renderOpenModal()
	if cached := m.renderOpenModal(); cached != first {
		t.Fatalf("cached prompt modal differs\ncached: %q\nfirst: %q", cached, first)
	}
	if !strings.Contains(first, "api/") || !strings.Contains(first, "界.http") {
		t.Fatalf("prompt modal lost suggestions: %q", first)
	}

	key := m.modalRender.key
	m.openPathPrompt.menu.Reset(nil)
	m.openPathError = "permission denied"
	withErr := m.renderOpenModal()
	if m.modalRender.key == key {
		t.Fatal("modal geometry change reused the old underlay snapshot")
	}
	if !strings.Contains(withErr, "permission denied") {
		t.Fatalf("modal geometry change lost its error: %q", withErr)
	}
	if cachedErr := m.renderOpenModal(); cachedErr != withErr {
		t.Fatalf("cached error frame differs\ncached: %q\nwant: %q", cachedErr, withErr)
	}
}

func TestModalRenderCacheInvalidation(t *testing.T) {
	m := newTestModelWithDoc(sampleRequestDoc)
	m.width = 80
	m.height = 24
	m.frameWidth = 82
	m.frameHeight = 26
	m.ready = true
	m.showNewFileModal = true
	_ = m.applyLayout()

	_ = m.View()
	if m.modalRender.screen == nil {
		t.Fatal("modal render did not populate the cache")
	}

	m.applyThemeDefinition(theme.DefaultDefinition())
	if m.modalRender.screen != nil {
		t.Fatal("theme change did not invalidate the modal cache")
	}

	_ = m.View()
	if m.modalRender.screen == nil {
		t.Fatal("modal render did not rebuild after a theme change")
	}

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	resized := updated.(Model)
	m = &resized
	if m.modalRender.screen != nil {
		t.Fatal("terminal resize did not invalidate the modal cache")
	}

	_ = m.View()
	m.closeNewFileModal()
	_ = m.View()
	if m.modalRender.screen != nil {
		t.Fatal("closing the modal did not discard the modal cache")
	}
}

func TestModalUnderlayFollowsBackgroundUpdates(t *testing.T) {
	msgs := []struct {
		name string
		msg  tea.Msg
	}{
		{"status message", statusMsg{text: "background work finished", level: statusInfo, noModal: true}},
		{"response loading tick", responseLoadingTickMsg{}},
		{"latency animation", latAnimMsg{}},
	}

	for _, tt := range msgs {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModelWithDoc(sampleRequestDoc)
			m.width, m.height = 80, 24
			m.frameWidth, m.frameHeight = 82, 26
			m.ready = true
			steadyFrame(m)
			m.showNewFileModal = true
			_ = m.applyLayout()
			_ = m.View()

			updated, _ := m.Update(tt.msg)
			next := updated.(Model)

			got := next.View()
			next.invalidateModalRender()
			if diff := frameDiff(got, next.View()); diff != "" {
				t.Fatalf("frame behind the modal went stale, %s", diff)
			}
		})
	}
}

func TestModalUnderlaySurvivesModalKeys(t *testing.T) {
	keys := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("r")},
		{Type: tea.KeyDown},
		{Type: tea.KeyTab},
		{Type: tea.KeyRunes, Runes: []rune("q")},
		{Type: tea.KeyEnter},
	}

	tests := []struct {
		name string
		open func(*testing.T, *Model)
	}{
		{"open", func(t *testing.T, m *Model) { openPathModal(t, m) }},
		{"new file", func(_ *testing.T, m *Model) { m.openNewFileModal() }},
		{"layout save", func(_ *testing.T, m *Model) { m.openLayoutSaveModal() }},
		{"file change", func(_ *testing.T, m *Model) { m.openFileChangeModal("changed on disk") }},
		{"theme selector", func(_ *testing.T, m *Model) { m.openThemeSelector() }},
		{"environment selector", func(_ *testing.T, m *Model) { m.openEnvironmentSelector() }},
		{"status", func(_ *testing.T, m *Model) { m.openStatusModal(statusError, "it broke") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(Config{WorkspaceRoot: completionWorkspace(t)})
			model.width, model.height = 100, 32
			model.frameWidth, model.frameHeight = 102, 34
			model.ready = true
			steadyFrame(&model)
			_ = model.applyLayout()
			tt.open(t, &model)
			_ = model.View()

			for i, key := range keys {
				updated, _ := model.Update(key)
				model = updated.(Model)

				got := model.View()
				model.invalidateModalRender()
				if diff := frameDiff(got, model.View()); diff != "" {
					t.Fatalf("frame after key %d (%v) came from a stale cache, %s", i, key, diff)
				}
			}
		})
	}
}

func TestModalUnderlayIsReusedWhileTyping(t *testing.T) {
	model := New(Config{WorkspaceRoot: completionWorkspace(t)})
	model.width, model.height = 100, 32
	model.frameWidth, model.frameHeight = 102, 34
	model.ready = true
	steadyFrame(&model)
	_ = model.applyLayout()
	openPathModal(t, &model)
	_ = model.View()

	for _, r := range "req" {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		model = updated.(Model)
		if model.modalRender.screen == nil {
			t.Fatalf("typing %q dropped the cached underlay", r)
		}
		_ = model.View()
	}
}

func frameDiff(got, want string) string {
	a, b := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(a), len(b)) {
		x, y := frameRowAt(a, i), frameRowAt(b, i)
		if x != y {
			return fmt.Sprintf("row %d\ngot:  %q\nwant: %q", i, x, y)
		}
	}
	return ""
}

func frameRowAt(rows []string, i int) string {
	if i < 0 || i >= len(rows) {
		return ""
	}
	return rows[i]
}

func BenchmarkOpenModalRender(b *testing.B) {
	m := newTestModelWithDoc(sampleRequestDoc)
	m.width = 120
	m.height = 40
	m.ready = true
	m.showOpenModal = true
	_ = m.applyLayout()
	for i := range 40 {
		m.openPathPrompt.menu.Reset(append(
			m.openPathPrompt.menu.Items(),
			prompt.Item{Label: fmt.Sprintf("request-%02d.http", i), Summary: "workspace file"},
		))
	}

	b.Run("uncached", func(b *testing.B) {
		for b.Loop() {
			m.invalidateModalRender()
			_ = m.renderOpenModal()
		}
	})
	b.Run("cached", func(b *testing.B) {
		_ = m.renderOpenModal()
		b.ResetTimer()
		for b.Loop() {
			_ = m.renderOpenModal()
		}
	})
}
