package clipboard

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

var (
	errNoTool = errors.New("no clipboard tool")
	overCap   = strings.Repeat("a", 1<<20)
)

func noTool(string) error {
	return errNoTool
}

type termWriter struct {
	writes []string
	err    error
}

func (w *termWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

func TestNewSendsToTerm(t *testing.T) {
	term := &termWriter{}
	c := New(term, 0)
	c.write = noTool
	sent, err := c.Write("a")
	if !sent || err != nil {
		t.Fatalf("Write() = %v, %v, want true, nil", sent, err)
	}
	if len(term.writes) != 1 || term.writes[0] != "\x1b]52;c;YQ==\x07" {
		t.Fatalf("term got %q, want one write of %q", term.writes, "\x1b]52;c;YQ==\x07")
	}
}

func TestNewKeepsLimit(t *testing.T) {
	term := &termWriter{}
	c := New(term, 11)
	c.write = noTool
	sent, err := c.Write("abc")
	var tooLarge *TooLargeError
	if sent || !errors.As(err, &tooLarge) || tooLarge.Limit != 11 {
		t.Fatalf("Write() = %v, %v, want false, a TooLargeError with limit 11", sent, err)
	}
	if len(term.writes) != 0 {
		t.Fatalf("term got %q, want no writes", term.writes)
	}
}

func TestWriteUsesTool(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "small", text: "abc"},
		{name: "over the cap", text: overCap},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			term := &termWriter{}
			c := Clipboard{term: term, write: func(text string) error {
				got = text
				return nil
			}}

			sent, err := c.Write(tt.text)
			if sent || err != nil {
				t.Fatalf("Write() = %v, %v, want false, nil", sent, err)
			}
			if got != tt.text {
				t.Fatalf("tool got %d bytes, want %d", len(got), len(tt.text))
			}
			if len(term.writes) != 0 {
				t.Fatalf("term got %d writes, want none", len(term.writes))
			}
		})
	}
}

func TestWriteSendsOSC52(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "one byte", text: "a", want: "\x1b]52;c;YQ==\x07"},
		{name: "two bytes", text: "ab", want: "\x1b]52;c;YWI=\x07"},
		{name: "three bytes", text: "abc", want: "\x1b]52;c;YWJj\x07"},
		{name: "utf-8", text: "héllo 👋", want: "\x1b]52;c;aMOpbGxvIPCfkYs=\x07"},
		{name: "newline", text: "\n", want: "\x1b]52;c;Cg==\x07"},
		{name: "crlf", text: "\r\n", want: "\x1b]52;c;DQo=\x07"},
		{name: "escape and bell", text: "a\x1b]0;x\x07b", want: "\x1b]52;c;YRtdMDt4B2I=\x07"},
		{name: "plus and slash", text: "~~~???", want: "\x1b]52;c;fn5+Pz8/\x07"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := &termWriter{}
			c := Clipboard{term: term, write: noTool}

			sent, err := c.Write(tt.text)
			if !sent || err != nil {
				t.Fatalf("Write() = %v, %v, want true, nil", sent, err)
			}
			if len(term.writes) != 1 || term.writes[0] != tt.want {
				t.Fatalf("term got %q, want one write of %q", term.writes, tt.want)
			}
			payload := strings.TrimSuffix(strings.TrimPrefix(term.writes[0], "\x1b]52;c;"), "\x07")
			got, err := base64.StdEncoding.DecodeString(payload)
			if err != nil || string(got) != tt.text {
				t.Fatalf("payload decodes to %q, %v, want %q", got, err, tt.text)
			}
		})
	}
}

func TestWriteWithoutTermReturnsToolError(t *testing.T) {
	c := Clipboard{write: noTool}

	sent, err := c.Write(overCap)
	if sent || !errors.Is(err, errNoTool) {
		t.Fatalf("Write() = %v, %v, want false, %v", sent, err, errNoTool)
	}
}

func TestWriteCapsSequence(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
		want  int64
	}{
		{name: "default", want: 1 << 20},
		{name: "16 KiB", limit: 16 << 10, want: 16 << 10},
		{name: "8 MiB", limit: 8 << 20, want: 8 << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fits := (int(tt.want) - len("\x1b]52;c;\x07")) / 4 * 3

			term := &termWriter{}
			c := Clipboard{term: term, limit: tt.limit, write: noTool}
			sent, err := c.Write(strings.Repeat("a", fits))
			if !sent || err != nil {
				t.Fatalf("Write() at the limit = %v, %v, want true, nil", sent, err)
			}
			if len(term.writes) != 1 || int64(len(term.writes[0])) != tt.want {
				t.Fatalf("term got %d writes, want one write of %d bytes", len(term.writes), tt.want)
			}

			term = &termWriter{}
			c = Clipboard{term: term, limit: tt.limit, write: noTool}
			sent, err = c.Write(strings.Repeat("a", fits+1))
			var tooLarge *TooLargeError
			if sent || !errors.As(err, &tooLarge) || tooLarge.Limit != tt.want {
				t.Fatalf("Write() over the limit = %v, %v, want false, TooLargeError{%d}", sent, err, tt.want)
			}
			if len(term.writes) != 0 {
				t.Fatalf("term got %d writes over the limit, want none", len(term.writes))
			}
		})
	}
}

func TestWriteReturnsTermError(t *testing.T) {
	termErr := errors.New("broken pipe")
	c := Clipboard{term: &termWriter{err: termErr}, write: noTool}

	sent, err := c.Write("abc")
	if sent || !errors.Is(err, termErr) {
		t.Fatalf("Write() = %v, %v, want false, %v", sent, err, termErr)
	}
}

func TestRead(t *testing.T) {
	tests := []struct {
		name string
		text string
		err  error
	}{
		{name: "text", text: "abc"},
		{name: "error", err: errNoTool},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Clipboard{read: func() (string, error) {
				return tt.text, tt.err
			}}

			got, err := c.Read()
			if got != tt.text || !errors.Is(err, tt.err) {
				t.Fatalf("Read() = %q, %v, want %q, %v", got, err, tt.text, tt.err)
			}
		})
	}
}
