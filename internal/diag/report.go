package diag

import "fmt"

// Pos identifies a source location. Line and Col are one-based when present.
type Pos struct {
	Path string
	Line int
	Col  int
}

func (p Pos) String() string {
	switch {
	case p.Path != "" && p.Line > 0 && p.Col > 0:
		return fmt.Sprintf("%s:%d:%d", p.Path, p.Line, p.Col)
	case p.Path != "" && p.Line > 0:
		return fmt.Sprintf("%s:%d", p.Path, p.Line)
	case p.Line > 0 && p.Col > 0:
		return fmt.Sprintf("%d:%d", p.Line, p.Col)
	case p.Line > 0:
		return fmt.Sprintf("%d", p.Line)
	default:
		return p.Path
	}
}

// Span identifies a source range for a diagnostic.
type Span struct {
	Start Pos
	End   Pos
	Label string
}

// Label describes an additional source annotation.
type Label struct {
	Span    Span
	Message string
	Primary bool
}

// NoteKind identifies secondary diagnostic text.
type NoteKind string

const (
	NoteInfo       NoteKind = "note"
	NoteHelp       NoteKind = "help"
	NoteWarning    NoteKind = "warning"
	NoteSuggestion NoteKind = "suggestion"
)

// Note describes supporting information such as a note, help, or suggestion.
type Note struct {
	Kind    NoteKind
	Span    Span
	Message string
}

// ChainKind identifies the role of one entry in an error chain.
type ChainKind string

const (
	ChainOperation ChainKind = "operation"
	ChainCause     ChainKind = "cause"
)

// ChainEntry describes one context or cause entry in an error chain.
type ChainEntry struct {
	Class     Class
	Component Component
	Kind      ChainKind
	Message   string
	Children  []ChainEntry
}

// StackFrame describes one runtime stack frame.
type StackFrame struct {
	Name string
	Pos  Pos
}

// Diagnostic is one diagnostic entry in a report.
type Diagnostic struct {
	Class     Class
	Component Component
	Severity  Severity
	Message   string
	Span      Span
	Labels    []Label
	Source    []byte
	Notes     []Note
	Chain     []ChainEntry
	Frames    []StackFrame
}

// Report is the structured diagnostic form used by renderers.
type Report struct {
	Path   string
	Source []byte
	Items  []Diagnostic
}

type reporter interface {
	Diagnostic() Report
}

// ReportOf converts err into a structured report.
func ReportOf(err error) Report {
	if err == nil {
		return Report{}
	}
	if rep := reportOf(err); len(rep.Items) > 0 {
		return prepareReport(rep)
	}
	return prepareReport(Report{Items: []Diagnostic{plainDiagnostic(err)}})
}

// ClassOf returns the dominant diagnostic class found in err.
func ClassOf(err error) Class {
	if err == nil {
		return ClassUnknown
	}
	var first Class
	collectClasses(err, func(class Class) bool {
		first = classOrUnknown(class)
		return false
	})
	if first == "" {
		return ClassUnknown
	}
	return first
}

// Classes returns unique diagnostic classes in err, preserving traversal order.
func Classes(err error) []Class {
	var out []Class
	seen := make(map[Class]struct{})
	collectClasses(err, func(class Class) bool {
		class = classOrUnknown(class)
		if _, ok := seen[class]; ok {
			return true
		}
		seen[class] = struct{}{}
		out = append(out, class)
		return true
	})
	return out
}

// HasClass reports whether err contains class.
func HasClass(err error, class Class) bool {
	target := classOrUnknown(class)
	found := false
	collectClasses(err, func(class Class) bool {
		found = classOrUnknown(class) == target
		return !found
	})
	return found
}

// Class returns the first non-unknown item class in the report.
func (r Report) Class() Class {
	for _, it := range r.Items {
		if class := classOrUnknown(it.Class); class != ClassUnknown {
			return class
		}
	}
	return ClassUnknown
}

// Summary returns a compact, stable error summary.
func (r Report) Summary() string {
	r = prepareReport(r)
	if len(r.Items) == 0 {
		return "error"
	}
	it := r.Items[0]
	msg := it.Message
	if msg == "" {
		msg = "operation failed"
	}
	if len(r.Items) > 1 {
		if it.Class == ClassParse {
			if it.Span.Start.Line > 0 {
				return fmt.Sprintf(
					"%d parse errors, first at line %d: %s",
					len(r.Items),
					it.Span.Start.Line,
					msg,
				)
			}
			return fmt.Sprintf("%d parse errors, first: %s", len(r.Items), msg)
		}
		if it.Span.Start.Line > 0 {
			return fmt.Sprintf(
				"%d diagnostics, first at line %d: %s",
				len(r.Items),
				it.Span.Start.Line,
				msg,
			)
		}
		return fmt.Sprintf("%d diagnostics, first: %s", len(r.Items), msg)
	}
	if it.Class == ClassParse {
		if it.Span.Start.Line > 0 {
			return fmt.Sprintf("parse error at line %d: %s", it.Span.Start.Line, msg)
		}
		return "parse error: " + msg
	}
	if it.Span.Start.Line > 0 {
		return fmt.Sprintf(
			"%s at line %d: %s",
			severityOrError(it.Severity),
			it.Span.Start.Line,
			msg,
		)
	}
	return msg
}

func reportOf(err error) Report {
	if err == nil {
		return Report{}
	}
	if e, ok := err.(*diagnosticError); ok {
		return reportFromDiagnosticError(e)
	}
	if rep, ok := err.(reporter); ok {
		return rep.Diagnostic()
	}
	if wrapped, ok := err.(errsUnwrapper); ok {
		var out Report
		for _, child := range wrapped.Unwrap() {
			rep := reportOf(child)
			if len(rep.Items) == 0 {
				continue
			}
			if out.Path == "" {
				out.Path = rep.Path
			}
			if len(out.Source) == 0 {
				out.Source = rep.Source
			}
			out.Items = append(out.Items, rep.Items...)
		}
		return out
	}
	if wrapped, ok := err.(errUnwrapper); ok {
		return reportOf(wrapped.Unwrap())
	}
	return Report{}
}

func reportFromDiagnosticError(e *diagnosticError) Report {
	if e == nil {
		return Report{}
	}
	if len(e.report.Items) > 0 {
		return prepareReport(e.report)
	}
	if _, ok := e.err.(errsUnwrapper); ok {
		return Report{Items: []Diagnostic{leafDiagnostic(e)}}
	}
	if child := reportOf(e.err); len(child.Items) > 0 {
		return withOperation(child, e)
	}
	return Report{Items: []Diagnostic{leafDiagnostic(e)}}
}

func withOperation(rep Report, e *diagnosticError) Report {
	rep = prepareReport(rep)
	if len(rep.Items) == 0 || e == nil {
		return rep
	}
	if class := classFromError(e); rep.Items[0].Class == ClassUnknown && class != ClassUnknown {
		rep.Items[0].Class = class
	}
	if rep.Items[0].Component == "" && e.component != "" {
		rep.Items[0].Component = e.component
	}
	if rep.Items[0].Component == "" && e.meta.component != "" {
		rep.Items[0].Component = e.meta.component
	}
	rep.Items[0].Chain = chainWithOp(
		operationEntry(e),
		chainOfError(e.err, rep.Items[0].Message, rep.Summary(), errorString(e.err)),
		rep.Items[0].Chain,
	)
	return rep
}

func leafDiagnostic(e *diagnosticError) Diagnostic {
	class := classFromError(e)
	if class == ClassUnknown {
		class = classOrUnknown(e.class)
	}
	component := e.component
	if component == "" {
		component = e.meta.component
	}
	msg := e.message
	if msg == "" && e.err != nil {
		if _, ok := e.err.(errsUnwrapper); ok {
			msg = string(class)
		} else {
			msg = e.err.Error()
		}
	}
	if msg == "" {
		msg = string(classOrUnknown(class))
	}
	if e.err != nil && isTransportFailure(class) && component == ComponentHTTP {
		msg = "request failed"
	}
	d := Diagnostic{
		Class:     classOrUnknown(class),
		Component: component,
		Severity:  SeverityError,
		Message:   msg,
		Span:      e.meta.span,
		Labels:    append([]Label(nil), e.meta.labels...),
		Source:    append([]byte(nil), e.meta.source...),
		Notes:     notesFromMetadata(e.meta),
		Chain:     opChain(e, msg),
		Frames:    append([]StackFrame(nil), e.meta.frames...),
	}
	if e.meta.path != "" && d.Span.Start.Path == "" {
		d.Span.Start.Path = e.meta.path
	}
	if e.err != nil && isTransportFailure(class) && component == ComponentHTTP {
		d.Notes = append(
			d.Notes,
			Note{Kind: NoteHelp, Message: "No response payload was received."},
		)
	}
	return d
}

func opChain(e *diagnosticError, msg string) []ChainEntry {
	if e == nil || e.err == nil {
		return nil
	}
	return chainWithOp(operationEntry(e), chainOfError(e.err, msg), nil)
}

func plainDiagnostic(err error) Diagnostic {
	class := classify(err)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if msg == "" {
		msg = string(class)
	}
	return Diagnostic{
		Class:    class,
		Severity: SeverityError,
		Message:  msg,
		Chain:    chainOfError(err, msg),
	}
}

func collectClasses(err error, visit func(Class) bool) bool {
	if err == nil {
		return true
	}
	if e, ok := err.(*diagnosticError); ok {
		if class := classFromError(e); class != ClassUnknown {
			if !visit(class) {
				return false
			}
		}
		for _, it := range e.report.Items {
			if !visit(it.Class) {
				return false
			}
		}
		return collectClasses(e.err, visit)
	}
	if rep, ok := err.(reporter); ok {
		for _, it := range rep.Diagnostic().Items {
			if !visit(it.Class) {
				return false
			}
		}
	}
	if class := classify(err); class != ClassUnknown {
		if !visit(class) {
			return false
		}
	}
	if wrapped, ok := err.(errsUnwrapper); ok {
		for _, child := range wrapped.Unwrap() {
			if !collectClasses(child, visit) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(errUnwrapper); ok {
		return collectClasses(wrapped.Unwrap(), visit)
	}
	return true
}

func notesFromMetadata(meta metadata) []Note {
	out := make([]Note, 0, len(meta.notes)+len(meta.help))
	for _, note := range meta.notes {
		out = append(out, Note{Kind: NoteInfo, Message: note})
	}
	for _, help := range meta.help {
		out = append(out, Note{Kind: NoteHelp, Message: help})
	}
	return out
}
