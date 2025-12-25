package rts

import "fmt"

type Pos struct {
	Path string
	Line int
	Col  int
}

func (p Pos) String() string {
	if p.Path == "" {
		return fmt.Sprintf("%d:%d", p.Line, p.Col)
	}
	return fmt.Sprintf("%s:%d:%d", p.Path, p.Line, p.Col)
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
}
