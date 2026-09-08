package textarea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func editorWith(value string) *Model {
	m := newTextArea()
	m.Prompt = ""
	m.ShowLineNumbers = false
	m.SetHeight(1)
	m.SetWidth(40)
	m.SetValue(value)
	return &m
}

func send(m *Model, key tea.KeyMsg) {
	updated, _ := m.Update(key)
	*m = updated
}

func TestArrowKeysCrossWholeGraphemes(t *testing.T) {
	for _, tc := range []struct {
		value string
		stops []int
	}{
		{"a⚙️z", []int{0, 1, 3, 4}},
		{"a👩‍💻z", []int{0, 1, 4, 5}},
		{"aéz", []int{0, 1, 2, 3}},
		{"a🇳🇴z", []int{0, 1, 3, 4}},
	} {
		t.Run(tc.value, func(t *testing.T) {
			m := editorWith(tc.value)
			m.SetCursor(0)
			var right []int
			for range len(tc.stops) {
				right = append(right, m.col)
				send(m, tea.KeyMsg{Type: tea.KeyRight})
			}
			if !equalInts(right, tc.stops) {
				t.Errorf("right stopped at %v, want %v", right, tc.stops)
			}

			m.SetCursor(len(m.value[0]))
			var left []int
			for range len(tc.stops) {
				left = append(left, m.col)
				send(m, tea.KeyMsg{Type: tea.KeyLeft})
			}
			slices := append([]int(nil), tc.stops...)
			reverse(slices)
			if !equalInts(left, slices) {
				t.Errorf("left stopped at %v, want %v", left, slices)
			}
		})
	}
}

func TestDeleteRemovesWholeGrapheme(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"variation selector", "a⚙️z", "az"},
		{"zwj sequence", "a👩‍💻z", "az"},
		{"combining accent", "aéz", "az"},
		{"regional indicators", "a🇳🇴z", "az"},
		{"skin tone", "a👍🏽z", "az"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := editorWith(tc.value)
			m.SetCursor(len(m.value[0]) - 1)
			send(m, tea.KeyMsg{Type: tea.KeyBackspace})
			if got := m.Value(); got != tc.want {
				t.Errorf("backspace left %q, want %q", got, tc.want)
			}
			if m.col != 1 {
				t.Errorf("backspace left cursor at %d, want 1", m.col)
			}

			m = editorWith(tc.value)
			m.SetCursor(1)
			send(m, tea.KeyMsg{Type: tea.KeyDelete})
			if got := m.Value(); got != tc.want {
				t.Errorf("delete left %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTransposeSwapsWholeGraphemes(t *testing.T) {
	m := editorWith("a⚙️z")
	m.SetCursor(1)
	send(m, tea.KeyMsg{Type: tea.KeyCtrlT})
	if got := m.Value(); got != "⚙️az" {
		t.Fatalf("transpose produced %q, want %q", got, "⚙️az")
	}
	if m.col != 3 {
		t.Fatalf("transpose left cursor at %d, want 3", m.col)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func reverse(s []int) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}
