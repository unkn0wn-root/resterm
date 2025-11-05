package rtfmt

import (
	"fmt"
	"io"
)

type ErrorHandler func(error)

// Fprintf writes to the writer and invokes the handler on failure.
func Fprintf(w io.Writer, format string, handler ErrorHandler, args ...any) error {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		if handler != nil {
			handler(err)
		}
		return err
	}
	return nil
}

// Fprintln mirrors fmt.Fprintln but adds error handling.
func Fprintln(w io.Writer, handler ErrorHandler, args ...any) error {
	if _, err := fmt.Fprintln(w, args...); err != nil {
		if handler != nil {
			handler(err)
		}
		return err
	}
	return nil
}

// LogHandler builds an ErrorHandler that logs using the provided function and
// captures the formatting arguments.
func LogHandler(logf func(string, ...any), format string, args ...any) ErrorHandler {
	if logf == nil {
		return nil
	}
	captured := append([]any(nil), args...)
	return func(err error) {
		logf(format, append(captured, err)...)
	}
}
