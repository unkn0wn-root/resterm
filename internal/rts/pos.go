package rts

import (
	"context"
	"fmt"
)

// File is one text that RTS code is read from, such as a module or the
// document that holds a script block. Every position read from the text points
// at the same File, so code and errors keep the revision they were read from.
// A host that knows only where a snippet sits may leave the text out.
type File struct {
	path string
	src  []byte
}

// NewFile returns the File for src read from path. src must not change after
// the call.
func NewFile(path string, src []byte) *File {
	return &File{path: path, src: src}
}

// Pos returns the position at line and col in f.
func (f *File) Pos(line, col int) Pos {
	return Pos{file: f, Line: line, Col: col}
}

// Start returns the position of the first byte of f.
func (f *File) Start() Pos {
	return f.Pos(1, 1)
}

// Pos is a line and column in a File. The zero Pos is in no file, so it has
// neither a path nor a text.
type Pos struct {
	file *File
	Line int
	Col  int
}

// Path returns the path of p's file, or "" when p has no file or the file has
// no path.
func (p Pos) Path() string {
	if p.file == nil {
		return ""
	}
	return p.file.path
}

// Source returns the text of p's file, or nil when the text is not known.
func (p Pos) Source() []byte {
	if p.file == nil {
		return nil
	}
	return p.file.src
}

// String returns "path:line:col", or "line:col" when p has no path.
func (p Pos) String() string {
	path := p.Path()
	if path == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Col)
	}
	return fmt.Sprintf("%s:%d:%d", path, p.Line, p.Col)
}

// A ParseError is a syntax error at Pos.
type ParseError struct {
	Pos Pos
	Msg string
}

// Error returns "Pos: Msg", or Msg alone when Pos has no line.
func (e *ParseError) Error() string {
	if e.Pos.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("%s: %s", e.Pos.String(), e.Msg)
}

// A RuntimeError is an error raised at Pos while code runs.
type RuntimeError struct {
	Pos Pos
	Msg string
}

// Error returns "Pos: Msg", or Msg alone when Pos has no line.
func (e *RuntimeError) Error() string {
	if e.Pos.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("%s: %s", e.Pos.String(), e.Msg)
}

// An AbortError is a RuntimeError that try cannot catch. Kind says why the run stopped.
type AbortError struct {
	*RuntimeError
	Kind AbortKind
}

// Unwrap reports a timeout as context.DeadlineExceeded and a cancel as
// context.Canceled, so errors.Is matches the context error that stopped the run.
func (e *AbortError) Unwrap() error {
	switch e.Kind {
	case AbortTimeout:
		return context.DeadlineExceeded
	case AbortCanceled:
		return context.Canceled
	}
	return nil
}

// An UndefinedNameError is the RuntimeError for reading a name that is not defined.
type UndefinedNameError struct {
	*RuntimeError
	Name string
}

// An AbortKind says why an AbortError stopped the run.
type AbortKind int

const (
	AbortScript   AbortKind = iota // the step limit, an unavailable host name, or a call to Abortf
	AbortTimeout                   // the time limit or the context deadline
	AbortCanceled                  // a canceled context
)
