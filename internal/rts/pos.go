package rts

import "fmt"

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

func (p Pos) String() string {
	path := p.Path()
	if path == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Col)
	}
	return fmt.Sprintf("%s:%d:%d", path, p.Line, p.Col)
}

type ParseError struct {
	Pos Pos
	Msg string
}

func (e *ParseError) Error() string {
	if e.Pos.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("%s: %s", e.Pos.String(), e.Msg)
}

type RuntimeError struct {
	Pos Pos
	Msg string
}

func (e *RuntimeError) Error() string {
	if e.Pos.Line == 0 {
		return e.Msg
	}
	return fmt.Sprintf("%s: %s", e.Pos.String(), e.Msg)
}

type AbortError struct {
	*RuntimeError
	Kind AbortKind
}

type UndefinedNameError struct {
	*RuntimeError
	Name string
}

type AbortKind int

const (
	AbortScript AbortKind = iota
	AbortTimeout
	AbortCanceled
)
