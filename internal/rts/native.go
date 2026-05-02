package rts

// NativeFunc is the Go implementation shape for an RTS built-in function.
// Implementations should validate args with Args so user-facing signatures and
// errors stay consistent.
type NativeFunc func(ctx *Ctx, pos Pos, args []Value) (Value, error)

// NativeNamed wraps f as a native value and records name in runtime stack traces.
func NativeNamed(name string, f NativeFunc) Value {
	return Native(func(ctx *Ctx, pos Pos, args []Value) (Value, error) {
		if ctx != nil {
			ctx.push(Frame{Kind: FrameNative, Pos: pos, Name: name})
			defer ctx.pop()
		}
		return f(ctx, pos, args)
	})
}
