package rtfmt

import (
	"fmt"
	"io"
)

type ErrorHandler func(error)

func Fprintf(w io.Writer, format string, handler ErrorHandler, args ...any) error {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		if handler != nil {
			handler(err)
		}
		return err
	}
	return nil
}

func Fprintln(w io.Writer, handler ErrorHandler, args ...any) error {
	if _, err := fmt.Fprintln(w, args...); err != nil {
		if handler != nil {
			handler(err)
		}
		return err
	}
	return nil
}

func LogHandler(logf func(string, ...any), format string, args ...any) ErrorHandler {
	if logf == nil {
		return nil
	}
	captured := append([]any(nil), args...)
	return func(err error) {
		logf(format, append(captured, err)...)
	}
}
