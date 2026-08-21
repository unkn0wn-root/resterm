package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func newStatusModalModel(width int) *Model {
	model := New(Config{})
	model.width = width
	model.height = 30
	model.ready = true
	model.statusUser = "tester"
	model.statusHost = "box"
	return &model
}

const (
	longStatusText   = "Request failed: dial tcp 10.0.0.1:8443: connect: connection refused after 3 retries"
	longerStatusText = longStatusText + " while resolving api.internal.example.com"
)

func TestLongWarningOpensStatusModal(t *testing.T) {
	m := newStatusModalModel(80)
	m.setStatusMessage(statusMsg{text: longStatusText, level: statusWarn})

	if !m.showStatusModal {
		t.Fatal("expected a truncated warning to open the status modal")
	}
	if m.statusModalMessage != longStatusText {
		t.Fatalf("expected the whole message in the modal, got %q", m.statusModalMessage)
	}
	if m.statusModalLevel != statusWarn {
		t.Fatalf("expected the modal to keep the warn level, got %v", m.statusModalLevel)
	}

	bar := ansi.Strip(m.renderStatusBar())
	if strings.Contains(bar, longStatusText) {
		t.Fatalf("expected the bar to keep truncating the message, got %q", bar)
	}
	if !strings.Contains(bar, "…") {
		t.Fatalf("expected the truncated bar text to end in an ellipsis, got %q", bar)
	}

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Warning") {
		t.Fatalf("expected a warning title, got %q", view)
	}
	if !strings.Contains(view, "refused after 3 retries") {
		t.Fatalf("expected the modal to show the end of the message, got %q", view)
	}
}

func TestShortWarningLeavesStatusModalClosed(t *testing.T) {
	m := newStatusModalModel(120)
	m.setStatusMessage(statusMsg{text: "No request at cursor", level: statusWarn})

	if m.showStatusModal {
		t.Fatalf("expected a warning that fits to stay in the bar, got modal %q", m.statusModalMessage)
	}
	if bar := ansi.Strip(m.renderStatusBar()); !strings.Contains(bar, "No request at cursor") {
		t.Fatalf("expected the bar to show the warning whole, got %q", bar)
	}
}

func TestStatusModalFollowsBarWidth(t *testing.T) {
	wide := newStatusModalModel(200)
	wide.setStatusMessage(statusMsg{text: longStatusText, level: statusWarn})
	if wide.showStatusModal {
		t.Fatalf("expected the message to fit a 200 column bar, got modal %q", wide.statusModalMessage)
	}

	narrow := newStatusModalModel(60)
	narrow.setStatusMessage(statusMsg{text: longStatusText, level: statusWarn})
	if !narrow.showStatusModal {
		t.Fatal("expected the message to overflow a 60 column bar")
	}
}

func TestStatusBarHoldsEveryLevelToItsShare(t *testing.T) {
	cases := []struct {
		name string
		msg  statusMsg
	}{
		{"warn", statusMsg{text: longerStatusText, level: statusWarn}},
		{"error", statusMsg{text: longerStatusText, level: statusError}},
		{"info", statusMsg{text: longerStatusText, level: statusInfo}},
		{"success", statusMsg{text: longerStatusText, level: statusSuccess}},
		{"run summary", statusMsg{text: longerStatusText, level: statusWarn, noModal: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newStatusModalModel(120)
			m.focus = focusEditor
			m.currentFile = "orders.http"
			m.setStatusMessage(tc.msg)

			plain := ansi.Strip(m.renderStatusBar())
			shown, ok := statusBarSegmentText(plain, longerStatusText)
			if !ok {
				t.Fatalf("expected the message on the bar, got %q", plain)
			}
			if share := lipgloss.Width(shown) * 100 / m.statusBarContentWidth(); share > statusBarStatusShare {
				t.Fatalf("expected the message to hold %d%% of the bar, took %d%% (%q)",
					statusBarStatusShare, share, shown)
			}
			if !strings.Contains(plain, "orders.http") {
				t.Fatalf("expected the file segment to survive beside the message, got %q", plain)
			}
		})
	}
}

func statusBarSegmentText(bar, want string) (string, bool) {
	for cut := len(want); cut > 0; cut-- {
		head := want[:cut]
		if idx := strings.Index(bar, head); idx >= 0 {
			shown := bar[idx:]
			if before, _, ok := strings.Cut(shown, "…"); ok {
				return before + "…", true
			}
			return head, true
		}
	}
	return "", false
}

func TestStatusBarFitsMatchesRenderedBar(t *testing.T) {
	for width := 20; width <= 200; width += 3 {
		m := newStatusModalModel(width)
		m.statusMessage = statusMsg{text: longStatusText, level: statusWarn}

		fits := m.statusBarFits(longStatusText, statusWarn)
		bar := ansi.Strip(m.renderStatusBar())
		if whole := strings.Contains(bar, longStatusText); fits != whole {
			t.Fatalf("width %d: statusBarFits=%v, bar shows it whole=%v (%q)", width, fits, whole, bar)
		}
	}
}

func TestStatusModalBodyIsNotBold(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	cases := []struct {
		name  string
		level statusLevel
	}{{"warn", statusWarn}, {"error", statusError}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newStatusModalModel(80)
			m.theme.Error = m.theme.Error.Bold(true)
			m.openStatusModal(tc.level, "connection refused")

			line := lineWith(m.renderStatusModal(), "connection refused")
			if line == "" {
				t.Fatal("expected the message on a line of its own")
			}
			if sgrBoldBefore(line, "connection refused") {
				t.Fatalf("expected the message to render without bold, got %q", line)
			}
		})
	}
}

func TestStatusModalPadsMessageAndInstructionsFromFrame(t *testing.T) {
	m := newStatusModalModel(80)
	m.openStatusModal(statusWarn, "connection refused")

	view := ansi.Strip(m.renderStatusModal())
	for _, text := range []string{"connection refused", "Dismiss"} {
		line := lineWith(view, text)
		if line == "" {
			t.Fatalf("expected modal line containing %q, got %q", text, view)
		}
		start := strings.Index(line, text)
		leftBorder := strings.LastIndex(line[:start], "│")
		if leftBorder < 0 {
			t.Fatalf("expected a left modal border before %q, got %q", text, line)
		}
		if pad := lipgloss.Width(line[leftBorder+len("│") : start]); pad < 2 {
			t.Fatalf("expected at least two cells before %q, got %d in %q", text, pad, line)
		}
	}
}

// Replay preceding SGR sequences to find the active bold state at needle.
func sgrBoldBefore(line, needle string) bool {
	idx := strings.Index(line, needle)
	if idx < 0 {
		return false
	}
	var state sgrState
	for i := 0; i < idx; {
		seq, width := ansiSequenceAt(line, i)
		if width == 0 {
			i++
			continue
		}
		state.apply(seq)
		i += width
	}
	return state.bold
}

func TestConfirmationWarningLeavesStatusModalClosed(t *testing.T) {
	tmp := t.TempDir()
	fileA := filepath.Join(tmp, "current.http")
	fileB := filepath.Join(tmp, "other.http")
	writeSampleFile(t, fileA, "### Local\n# @name Local\nGET https://local.test\n")
	writeSampleFile(t, fileB, "### Remote\n# @name Remote\nGET https://remote.test\n")

	model := New(Config{WorkspaceRoot: tmp, FilePath: fileA})
	m := &model
	m.width = 120
	m.height = 30
	m.ready = true
	if cmd := m.openFile(fileA); cmd != nil {
		cmd()
	}
	m.focus = focusFile
	selectNavigatorID(t, m, "file:"+fileB)
	m.markDirty()

	m = applyModelUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(m.statusMessage.text, "again") {
		t.Fatalf("expected a repeat-to-confirm warning, got %q", m.statusMessage.text)
	}
	if m.statusBarFits(m.statusMessage.text, m.statusMessage.level) {
		t.Fatalf("expected the confirmation to outgrow the bar, got %q", m.statusMessage.text)
	}
	if m.showStatusModal {
		t.Fatalf("expected the confirmation to stay in the bar, got modal %q", m.statusModalMessage)
	}
}

func TestShowStatusMessageKeyOpensAnyLevel(t *testing.T) {
	m := newStatusModalModel(80)
	saved := "Saved response body (1.2 MB) to /Users/dev/work/exports/2026/orders-response.json"
	m.setStatusMessage(statusMsg{text: saved, level: statusInfo})
	if m.showStatusModal {
		t.Fatal("expected info to stay in the bar until asked for")
	}

	updated := applyModelUpdate(t, m, keyMsgFor("g"))
	updated = applyModelUpdate(t, updated, keyMsgFor("."))

	if !updated.showStatusModal {
		t.Fatal("expected g . to open the status message")
	}
	if updated.statusModalMessage != saved {
		t.Fatalf("expected the whole message, got %q", updated.statusModalMessage)
	}
	if title := statusModalTitle(updated.statusModalLevel); title != "Status" {
		t.Fatalf("expected an info message to read as Status, got %q", title)
	}
}

func TestShortErrorStillOpensStatusModal(t *testing.T) {
	m := newStatusModalModel(120)
	m.setStatusMessage(statusMsg{text: "open failed", level: statusError})

	if !m.showStatusModal {
		t.Fatal("expected every error to open the status modal")
	}
	if title := statusModalTitle(m.statusModalLevel); title != "Error" {
		t.Fatalf("expected an error title, got %q", title)
	}
}

func TestNoModalWarningStaysInBar(t *testing.T) {
	m := newStatusModalModel(60)
	m.setStatusMessage(statusMsg{text: longStatusText, level: statusWarn, noModal: true})

	if m.showStatusModal {
		t.Fatalf("expected noModal to hold the warning in the bar, got modal %q", m.statusModalMessage)
	}
}

func TestInfoStatusNeverOpensStatusModal(t *testing.T) {
	m := newStatusModalModel(60)
	for _, level := range []statusLevel{statusInfo, statusSuccess} {
		m.setStatusMessage(statusMsg{text: longStatusText, level: level})
		if m.showStatusModal {
			t.Fatalf("expected level %v to stay in the bar, got modal %q", level, m.statusModalMessage)
		}
	}
}

func TestStatusBarFitsWithoutWidth(t *testing.T) {
	m := New(Config{})
	m.setStatusMessage(statusMsg{text: longStatusText, level: statusWarn})

	if m.showStatusModal {
		t.Fatal("expected no modal before the first window size is known")
	}
}

func TestStatusModalScrollsLongMessage(t *testing.T) {
	m := newStatusModalModel(80)
	m.setStatusMessage(statusMsg{
		text:  strings.TrimSpace(strings.Repeat("failure detail that runs on and on. ", 60)),
		level: statusError,
	})

	box := m.renderStatusModal()
	if height := lipgloss.Height(box); height > m.height {
		t.Fatalf("expected the modal to stay within %d rows, got %d", m.height, height)
	}
	if plain := ansi.Strip(box); !strings.Contains(plain, "j/k Scroll") {
		t.Fatalf("expected a scroll hint on an overlong message, got %q", plain)
	}

	before := m.statusModalViewport.YOffset
	updated, _ := m.Update(keyMsgFor("j"))
	scrolled := updated.(Model)
	if scrolled.statusModalViewport.YOffset == before {
		t.Fatal("expected j to scroll the status modal")
	}

	updated, _ = scrolled.Update(keyMsgFor("esc"))
	if updated.(Model).showStatusModal {
		t.Fatal("expected esc to dismiss the status modal")
	}
}
