package diag

import (
	"errors"
	"fmt"
)

// Class identifies the stable failure class for a diagnostic.
type Class string

const (
	ClassUnknown    Class = "unknown"
	ClassConfig     Class = "config"
	ClassParse      Class = "parse"
	ClassTimeout    Class = "timeout"
	ClassCanceled   Class = "canceled"
	ClassNetwork    Class = "network"
	ClassTLS        Class = "tls"
	ClassAuth       Class = "auth"
	ClassProtocol   Class = "protocol"
	ClassRoute      Class = "route"
	ClassFilesystem Class = "filesystem"
	ClassScript     Class = "script"
	ClassHistory    Class = "history"
	ClassUI         Class = "ui"
	ClassInternal   Class = "internal"
)

// Component identifies where a diagnostic originated.
type Component string

const (
	ComponentHTTP        Component = "http"
	ComponentGRPC        Component = "grpc"
	ComponentWebSocket   Component = "websocket"
	ComponentSSE         Component = "sse"
	ComponentOAuth       Component = "oauth"
	ComponentAuthCommand Component = "authcmd"
	ComponentHistory     Component = "history"
	ComponentRTS         Component = "rts"
	ComponentParser      Component = "parser"
	ComponentUI          Component = "ui"
)

// Severity controls how a diagnostic should be presented.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityNote    Severity = "note"
)

// Option configures a diagnostic error or report item.
type Option func(*metadata)

type metadata struct {
	component Component
	span      Span
	source    []byte
	path      string
	labels    []Label
	notes     []string
	help      []string
	frames    []StackFrame
}

// WithComponent records the package or protocol area that produced a diagnostic.
func WithComponent(component Component) Option {
	return func(m *metadata) {
		m.component = component
	}
}

// WithSpan records the source range associated with a diagnostic.
func WithSpan(span Span) Option {
	return func(m *metadata) {
		m.span = span
	}
}

// WithSource records source text used by renderers for span excerpts.
func WithSource(path string, src []byte) Option {
	return func(m *metadata) {
		m.path = path
		m.source = append([]byte(nil), src...)
	}
}

// WithLabel adds an additional source annotation.
func WithLabel(label Label) Option {
	return func(m *metadata) {
		m.labels = append(m.labels, label)
	}
}

// WithNote adds supporting diagnostic text.
func WithNote(text string) Option {
	return func(m *metadata) {
		m.notes = append(m.notes, text)
	}
}

// WithHelp adds actionable remediation text.
func WithHelp(text string) Option {
	return func(m *metadata) {
		m.help = append(m.help, text)
	}
}

// WithStack records runtime stack frames associated with a diagnostic.
func WithStack(frames []StackFrame) Option {
	return func(m *metadata) {
		m.frames = append([]StackFrame(nil), frames...)
	}
}

type diagnosticError struct {
	class     Class
	component Component
	message   string
	err       error
	report    Report
	meta      metadata
}

// New returns a classified diagnostic error.
func New(class Class, message string, opts ...Option) error {
	msg := messageText(message)
	if msg == "" {
		msg = string(classOrUnknown(class))
	}
	return newError(class, msg, nil, opts...)
}

// Newf returns a classified diagnostic error with a formatted message.
func Newf(class Class, format string, args ...any) error {
	return New(class, fmt.Sprintf(format, args...))
}

// Wrap records operation context on err while preserving err for errors.Is/As.
func Wrap(err error, operation string, opts ...Option) error {
	if err == nil {
		return nil
	}
	return newError(ClassUnknown, operation, err, opts...)
}

// Wrapf records formatted operation context on err.
func Wrapf(err error, format string, args ...any) error {
	return Wrap(err, fmt.Sprintf(format, args...))
}

// WrapAs records operation context and overrides the resulting diagnostic class.
func WrapAs(class Class, err error, operation string, opts ...Option) error {
	if err == nil {
		return nil
	}
	return newError(class, operation, err, opts...)
}

// WrapAsf records formatted operation context and overrides the diagnostic class.
func WrapAsf(class Class, err error, format string, args ...any) error {
	return WrapAs(class, err, fmt.Sprintf(format, args...))
}

// Join joins non-nil errors and annotates the aggregate.
func Join(class Class, message string, errs ...error) error {
	err := errors.Join(errs...)
	if err == nil {
		return nil
	}
	if message == "" {
		message = string(classOrUnknown(class))
	}
	return newError(class, message, err)
}

// FromReport returns err unchanged for an empty report, otherwise a diagnostic error.
func FromReport(rep Report, err error) error {
	rep = prepareReport(rep)
	if len(rep.Items) == 0 {
		return err
	}
	return &diagnosticError{
		class:     rep.Class(),
		component: rep.Items[0].Component,
		message:   rep.Summary(),
		err:       err,
		report:    rep,
	}
}

func newError(class Class, message string, err error, opts ...Option) error {
	var meta metadata
	for _, opt := range opts {
		if opt != nil {
			opt(&meta)
		}
	}
	return &diagnosticError{
		class:     classOrUnknown(class),
		component: meta.component,
		message:   messageText(message),
		err:       err,
		meta:      meta,
	}
}

func (e *diagnosticError) Error() string {
	switch {
	case e == nil:
		return ""
	case len(e.report.Items) > 0:
		return e.report.Summary()
	case e.err != nil && e.message != "":
		return fmt.Sprintf("%s: %s", e.message, e.err)
	case e.err != nil:
		return e.err.Error()
	case e.message != "":
		return e.message
	default:
		return string(classOrUnknown(e.class))
	}
}

func (e *diagnosticError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *diagnosticError) Diagnostic() Report {
	if e == nil {
		return Report{}
	}
	return reportFromDiagnosticError(e)
}

func classFromError(e *diagnosticError) Class {
	if e == nil {
		return ClassUnknown
	}
	if e.class != "" && e.class != ClassUnknown {
		return e.class
	}
	return classify(e.err)
}
