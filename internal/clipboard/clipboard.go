package clipboard

import (
	"encoding/base64"
	"errors"
	"io"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxSeq = 1 << 20
	frame  = len(ansi.ResetSystemClipboard)
)

// ErrTooLarge reports text whose OSC 52 sequence would exceed 1 MiB.
var ErrTooLarge = errors.New("text too large for OSC 52")

type Clipboard struct {
	term  io.Writer
	write func(string) error
	read  func() (string, error)
}

// New returns a Clipboard that falls back to OSC 52 on term. A nil term turns the fallback off.
func New(term io.Writer) Clipboard {
	return Clipboard{term: term, write: clipboard.WriteAll, read: clipboard.ReadAll}
}

// Write falls back to OSC 52 when the clipboard tool fails. sent means the
// sequence was written to term, not that the terminal accepted it.
func (c Clipboard) Write(text string) (sent bool, err error) {
	err = c.write(text)
	if err == nil || c.term == nil {
		return false, err
	}
	if frame+base64.StdEncoding.EncodedLen(len(text)) > maxSeq {
		return false, ErrTooLarge
	}
	if _, err = io.WriteString(c.term, ansi.SetSystemClipboard(text)); err != nil {
		return false, err
	}
	return true, nil
}

func (c Clipboard) Read() (string, error) {
	return c.read()
}
