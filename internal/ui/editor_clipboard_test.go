package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/unkn0wn-root/resterm/internal/clipboard"
	"github.com/unkn0wn-root/resterm/internal/config"
)

type recordingClipboard struct {
	writes []string
	sent   bool
	text   string
	err    error
}

func (c *recordingClipboard) Write(text string) (bool, error) {
	c.writes = append(c.writes, text)
	return c.sent, c.err
}

func (c *recordingClipboard) Read() (string, error) {
	return c.text, c.err
}

func TestNewSendsYankToTerminal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	yank := func(cfg Config) *statusMsg {
		model := New(cfg)
		model.editor.SetValue("abc")
		model.editor.moveCursorTo(0, 0)
		_ = model.setFocus(focusEditor)
		_ = model.setInsertMode(false, false)
		sendKeys(t, &model, "v", "l", "l")
		return editorEventFromCmd(t, model.handleKey(keyMsgFor("y"))).status
	}

	var term strings.Builder
	status := yank(Config{Terminal: &term})
	want := "Copied selection. Sent via OSC 52"
	if status == nil || status.level != statusInfo || status.text != want {
		t.Fatalf("status with a terminal = %+v, want %q", status, want)
	}
	if term.String() != "\x1b]52;c;YWJj\x07" {
		t.Fatalf("terminal got %q, want %q", term.String(), "\x1b]52;c;YWJj\x07")
	}

	status = yank(Config{})
	if status == nil || status.level != statusWarn || status.text != clipboardFallbackStatus {
		t.Fatalf("status without a terminal = %+v, want %q", status, clipboardFallbackStatus)
	}

	var limit config.ByteSize
	if err := limit.UnmarshalText([]byte("8")); err != nil {
		t.Fatal(err)
	}
	term.Reset()
	status = yank(Config{
		Terminal: &term,
		Settings: config.Settings{Clipboard: config.ClipboardSettings{OSC52Limit: &limit}},
	})
	want = "Too large for OSC 52 (limit 8 B). Saved in editor register"
	if status == nil || status.level != statusWarn || status.text != want {
		t.Fatalf("status over the limit = %+v, want %q", status, want)
	}
	if term.Len() != 0 {
		t.Fatalf("terminal got %q over the limit, want nothing", term.String())
	}
}

func TestYankSelectionClipboardStatus(t *testing.T) {
	tests := []struct {
		name  string
		clip  *recordingClipboard
		level statusLevel
		want  string
	}{
		{name: "tool", clip: &recordingClipboard{}, level: statusInfo, want: "Copied selection"},
		{
			name:  "osc 52",
			clip:  &recordingClipboard{sent: true},
			level: statusInfo,
			want:  "Copied selection. Sent via OSC 52",
		},
		{
			name:  "too large",
			clip:  &recordingClipboard{err: &clipboard.TooLargeError{Limit: 1 << 20}},
			level: statusWarn,
			want:  "Too large for OSC 52 (limit 1 MiB). Saved in editor register",
		},
		{
			name:  "too large for a raised limit",
			clip:  &recordingClipboard{err: &clipboard.TooLargeError{Limit: 8 << 20}},
			level: statusWarn,
			want:  "Too large for OSC 52 (limit 8 MiB). Saved in editor register",
		},
		{
			name:  "unavailable",
			clip:  &recordingClipboard{err: errors.New("no clipboard tool")},
			level: statusWarn,
			want:  clipboardFallbackStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := newTestEditor("abc")
			editor.clip = tt.clip
			editor.moveCursorTo(0, 0)
			editor, _ = editor.ToggleVisual()
			editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRight})
			editor, _ = editor.Update(tea.KeyMsg{Type: tea.KeyRight})

			editor, cmd := editor.YankSelection()
			evt := editorEventFromCmd(t, cmd)
			if evt.status == nil || evt.status.level != tt.level || evt.status.text != tt.want {
				t.Fatalf("status = %+v, want level %v text %q", evt.status, tt.level, tt.want)
			}
			if !slices.Equal(tt.clip.writes, []string{"abc"}) {
				t.Fatalf("clipboard writes = %q, want [abc]", tt.clip.writes)
			}
			if editor.registerText != "abc" {
				t.Fatalf("register = %q, want abc", editor.registerText)
			}
		})
	}
}

func TestDeleteCurrentLineSentViaOSC52(t *testing.T) {
	clip := &recordingClipboard{sent: true}
	editor := newTestEditor("alpha\nbeta")
	editor.clip = clip
	editor.moveCursorTo(0, 0)

	editor, cmd := editor.DeleteCurrentLine()
	evt := editorEventFromCmd(t, cmd)
	if evt.status == nil || evt.status.level != statusInfo || evt.status.text != "Deleted line. Sent via OSC 52" {
		t.Fatalf("status = %+v, want Deleted line. Sent via OSC 52", evt.status)
	}
	if !slices.Equal(clip.writes, []string{"alpha\n"}) {
		t.Fatalf("clipboard writes = %q, want [alpha\\n]", clip.writes)
	}
	if editor.registerText != "alpha\n" {
		t.Fatalf("register = %q, want alpha\\n", editor.registerText)
	}
}

func TestDeleteSelectionSentViaOSC52(t *testing.T) {
	clip := &recordingClipboard{sent: true}
	editor := newTestEditor("alpha")
	editor.clip = clip
	editor.moveCursorTo(0, 0)
	editor.startSelection(editor.caretPosition(), selectionManual)
	editor.selection.Update(cursorPosition{Line: 0, Column: 5, Offset: 5})
	editor.applySelectionHighlight()

	_, cmd := editor.DeleteSelection()
	evt := editorEventFromCmd(t, cmd)
	want := "Selection deleted. Sent via OSC 52"
	if evt.status == nil || evt.status.level != statusInfo || evt.status.text != want {
		t.Fatalf("status = %+v, want %q", evt.status, want)
	}
	if !slices.Equal(clip.writes, []string{"alpha"}) {
		t.Fatalf("clipboard writes = %q, want [alpha]", clip.writes)
	}
}

func TestPasteClipboardStatus(t *testing.T) {
	tests := []struct {
		name  string
		clip  *recordingClipboard
		level statusLevel
		want  string
		value string
	}{
		{name: "clipboard", clip: &recordingClipboard{text: "XY"}, level: statusInfo, want: "Pasted", value: "aXYbc"},
		{
			name:  "register",
			clip:  &recordingClipboard{err: errors.New("no clipboard tool")},
			level: statusWarn,
			want:  "Clipboard unavailable; pasted from editor register",
			value: "aZZbc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editor := newTestEditor("abc")
			editor.clip = tt.clip
			editor.registerText = "ZZ"
			editor.moveCursorTo(0, 0)

			editor, cmd := editor.PasteClipboard(true)
			evt := editorEventFromCmd(t, cmd)
			if evt.status == nil || evt.status.level != tt.level || evt.status.text != tt.want {
				t.Fatalf("status = %+v, want level %v text %q", evt.status, tt.level, tt.want)
			}
			if got := editor.Value(); got != tt.value {
				t.Fatalf("value = %q, want %q", got, tt.value)
			}
		})
	}
}

func TestSentWithoutMessageStaysEmpty(t *testing.T) {
	editor := newTestEditor("abc")
	editor.clip = &recordingClipboard{sent: true}

	status := editor.writeClipboardWithFallback("abc", "")
	if status.level != statusInfo || status.text != "" {
		t.Fatalf("status = %+v, want empty info", status)
	}
}

func TestEmptyClipboardTextSkipsWrite(t *testing.T) {
	clip := &recordingClipboard{sent: true}
	editor := newTestEditor("abc")
	editor.clip = clip

	evt := editorEventFromCmd(t, editor.copyToClipboard("", ""))
	if evt.status != nil {
		t.Fatalf("copy status = %+v, want none", evt.status)
	}
	status := editor.writeClipboardWithFallback("", "Deleted line")
	if status.text != "Deleted line" {
		t.Fatalf("status = %+v, want Deleted line", status)
	}
	if len(clip.writes) != 0 {
		t.Fatalf("clipboard writes = %q, want none", clip.writes)
	}
}
