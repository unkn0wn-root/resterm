package directive

import "fmt"

// Continuation identifies how a directive argument may span lines.
type Continuation uint8

const (
	// ContinueNone disables multiline arguments.
	ContinueNone Continuation = iota
	// ContinueOptions continues for unclosed option quotes or brackets.
	ContinueOptions
	// ContinueExpr continues for unclosed RestermScript groups.
	ContinueExpr
	// ContinueCapture continues for unclosed template markers or RTS groups.
	ContinueCapture
)

// Continuation returns the continuation mode for n.
// Unknown names use ContinueNone.
func (n Name) Continuation() Continuation {
	spec, ok := index[n]
	if !ok {
		return ContinueNone
	}
	return spec.Continues
}

// UnclosedError reports a directive argument with a missing delimiter.
type UnclosedError struct {
	Directive Name
	Closer    string
}

func (e *UnclosedError) Error() string {
	return fmt.Sprintf("%s is missing a closing %q", e.Directive.Tag(), e.Closer)
}
