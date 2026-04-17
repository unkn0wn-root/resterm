package headless

import "errors"

// UsageError reports invalid input or options passed to the headless API.
type UsageError struct {
	err error
}

var (
	// ErrNoFilePath reports that Options.FilePath was empty.
	ErrNoFilePath = errors.New("headless: file path is required")

	// ErrTooFewTargets reports that compare mode was enabled without enough targets.
	ErrTooFewTargets = errors.New("headless: compare requires at least two target environments")

	// ErrNilReport reports an attempt to encode a nil report.
	ErrNilReport = errors.New("headless: nil report")

	// ErrNilWriter reports an attempt to write to a nil writer.
	ErrNilWriter = errors.New("headless: nil writer")
)

func (e UsageError) Error() string {
	if e.err == nil {
		return "usage error"
	}
	return e.err.Error()
}

func (e UsageError) Unwrap() error {
	return e.err
}

// IsUsageError reports whether err contains a UsageError.
func IsUsageError(err error) bool {
	var target UsageError
	return errors.As(err, &target)
}
