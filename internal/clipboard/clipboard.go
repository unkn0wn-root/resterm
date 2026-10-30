package clipboard

import (
	"cmp"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultLimit = 1 << 20
	frame        = len(ansi.ResetSystemClipboard)
)

// TooLargeError reports text whose OSC 52 sequence would exceed Limit bytes.
type TooLargeError struct {
	Limit int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("OSC 52 sequence exceeds the %d byte limit", e.Limit)
}

type Clipboard struct {
	term  io.Writer
	limit int64
	write func(string) error
	read  func() (string, error)
}

// New returns a Clipboard that falls back to OSC 52 on term. A nil term turns the fallback off.
// limit caps the OSC 52 sequence in bytes, and zero means 1 MiB.
func New(term io.Writer, limit int64) Clipboard {
	return Clipboard{term: term, limit: limit, write: clipboard.WriteAll, read: clipboard.ReadAll}
}

// Write falls back to OSC 52 when the clipboard tool fails. sent means the
// sequence was written to term, not that the terminal accepted it.
func (c Clipboard) Write(text string) (sent bool, err error) {
	err = c.write(text)
	if err == nil || c.term == nil {
		return false, err
	}
	limit := cmp.Or(c.limit, defaultLimit)
	if int64(frame+base64.StdEncoding.EncodedLen(len(text))) > limit {
		return false, &TooLargeError{Limit: limit}
	}
	if _, err = io.WriteString(c.term, ansi.SetSystemClipboard(text)); err != nil {
		return false, err
	}
	return true, nil
}

func (c Clipboard) Read() (string, error) {
	return c.read()
}
