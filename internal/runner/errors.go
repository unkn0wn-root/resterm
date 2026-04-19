package runner

import (
	"errors"
	"fmt"
)

// ErrNilWriter reports an attempt to write a report to a nil io.Writer.
var ErrNilWriter = errors.New("runner: nil writer")

// ErrNilContext reports an attempt to run with a nil context.
var ErrNilContext = errors.New("runner: nil context")

type UsageError struct {
	err error
}

func usageError(format string, args ...any) error {
	return UsageError{err: fmt.Errorf(format, args...)}
}

func (e UsageError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e UsageError) Unwrap() error {
	return e.err
}

func IsUsageError(err error) bool {
	var target UsageError
	return errors.As(err, &target)
}
