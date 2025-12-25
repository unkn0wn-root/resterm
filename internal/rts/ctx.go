package rts

import (
	"context"
	"fmt"
	"time"
)

type Limits struct {
	MaxSteps int
	MaxCall  int
	MaxStr   int
	MaxList  int
	MaxDict  int
	Timeout  time.Duration
}

type FrameKind int

const (
	FrameFn FrameKind = iota
	FrameNative
	FrameExpr
)

type Frame struct {
	Kind FrameKind
	Pos  Pos
	Name string
}

type StackError struct {
	Err    error
	Frames []Frame
}

func (e *StackError) Error() string {
	return e.Err.Error()
}

func (e *StackError) Pretty() string {
	out := e.Err.Error()
	for _, f := range e.Frames {
		name := f.Name
		if name == "" {
			name = "<fn>"
		}
		out += fmt.Sprintf("\n  at %s in %s", f.Pos.String(), name)
	}
	return out
}

type Ctx struct {
	Ctx         context.Context
	Lim         Limits
	Now         func() time.Time
	UUID        func() (string, error)
	ReadFile    func(path string) ([]byte, error)
	BaseDir     string
	AllowRandom bool

	steps int
	depth int
	start time.Time
	stack []Frame
}

func NewCtx(ctx context.Context, lim Limits) *Ctx {
	if ctx == nil {
		ctx = context.Background()
	}

	c := &Ctx{Ctx: ctx, Lim: lim}
	c.Now = time.Now
	c.start = c.Now()
	return c
}

func (c *Ctx) Clone() *Ctx {
	if c == nil {
		return NewCtx(context.Background(), Limits{})
	}

	n := NewCtx(c.Ctx, c.Lim)
	if c.Now != nil {
		n.Now = c.Now
		n.start = n.Now()
	}

	n.UUID = c.UUID
	n.ReadFile = c.ReadFile
	n.BaseDir = c.BaseDir
	n.AllowRandom = c.AllowRandom
	return n
}

func (c *Ctx) CloneNoIO() *Ctx {
	n := c.Clone()
	n.ReadFile = nil
	n.BaseDir = ""
	return n
}

func (c *Ctx) tick(pos Pos) error {
	c.steps++
	if c.Lim.MaxSteps > 0 && c.steps > c.Lim.MaxSteps {
		return rtAbort(c, pos, "step limit exceeded")
	}
	if c.Lim.Timeout > 0 && c.Now().Sub(c.start) > c.Lim.Timeout {
		return rtAbort(c, pos, "timeout exceeded")
	}
	select {
	case <-c.Ctx.Done():
		return rtAbort(c, pos, "canceled: %v", c.Ctx.Err())
	default:
		return nil
	}
}

func (c *Ctx) push(f Frame) {
	c.stack = append(c.stack, f)
}

func (c *Ctx) pop() {
	if len(c.stack) == 0 {
		return
	}
	c.stack = c.stack[:len(c.stack)-1]
}

func rtErr(ctx *Ctx, pos Pos, format string, args ...any) error {
	base := &RuntimeError{Pos: pos, Msg: fmt.Sprintf(format, args...)}
	if ctx == nil {
		return base
	}
	frames := append([]Frame(nil), ctx.stack...)
	return &StackError{Err: base, Frames: frames}
}

func rtAbort(ctx *Ctx, pos Pos, format string, args ...any) error {
	base := &RuntimeError{Pos: pos, Msg: fmt.Sprintf(format, args...)}
	abort := &AbortError{RuntimeError: base}
	if ctx == nil {
		return abort
	}
	frames := append([]Frame(nil), ctx.stack...)
	return &StackError{Err: abort, Frames: frames}
}

func isAbort(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*AbortError); ok {
		return true
	}
	if se, ok := err.(*StackError); ok {
		_, ok := se.Err.(*AbortError)
		return ok
	}
	return false
}

func wrapErr(ctx *Ctx, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*StackError); ok {
		return err
	}
	if ctx == nil {
		return err
	}
	frames := append([]Frame(nil), ctx.stack...)
	return &StackError{Err: err, Frames: frames}
}
