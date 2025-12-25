package rts

import (
	"context"
	"fmt"
	"math"
	"sort"
)

type VM struct {
	ctx *Ctx
}

type Comp struct {
	Mod *Mod
	Env *Env
	Exp map[string]Value
}

type ret struct {
	v Value
}

func (r ret) Error() string { return "return" }

type breakSig struct {
	pos Pos
}

func (b breakSig) Error() string { return "break" }

type continueSig struct {
	pos Pos
}

func (c continueSig) Error() string { return "continue" }

func Exec(ctx *Ctx, mod *Mod, pre map[string]Value) (*Comp, error) {
	if mod == nil {
		return nil, fmt.Errorf("nil module")
	}
	if ctx == nil {
		ctx = NewCtx(context.Background(), Limits{})
	}
	vm := &VM{ctx: ctx}
	env := NewEnv(nil)
	for k, v := range pre {
		env.DefConst(k, v)
	}
	exp := map[string]Value{}
	for _, st := range mod.Stmts {
		if err := vm.execStmt(env, exp, st); err != nil {
			if _, ok := err.(ret); ok {
				return nil, rtErr(ctx, st.Pos(), "return outside fn")
			}
			if b, ok := err.(breakSig); ok {
				return nil, rtErr(ctx, b.pos, "break outside loop")
			}
			if c, ok := err.(continueSig); ok {
				return nil, rtErr(ctx, c.pos, "continue outside loop")
			}
			return nil, err
		}
	}
	return &Comp{Mod: mod, Env: env, Exp: exp}, nil
}

func (vm *VM) execStmt(env *Env, exp map[string]Value, st Stmt) error {
	if err := vm.tick(st.Pos()); err != nil {
		return err
	}
	switch s := st.(type) {
	case *LetStmt:
		if env.HasLocal(s.Name) {
			return rtErr(vm.ctx, s.Pos(), "name already defined: %q", s.Name)
		}
		v, err := vm.eval(env, s.Val)
		if err != nil {
			return err
		}
		if err := vm.chkVal(s.Pos(), v); err != nil {
			return err
		}
		if s.Const {
			env.DefConst(s.Name, v)
		} else {
			env.Def(s.Name, v)
		}
		if s.Exported && exp != nil {
			exp[s.Name] = v
		}
		return nil
	case *AssignStmt:
		v, err := vm.eval(env, s.Val)
		if err != nil {
			return err
		}
		if err := vm.chkVal(s.Pos(), v); err != nil {
			return err
		}
		found, isConst := env.Set(s.Name, v)
		if !found {
			return rtErr(vm.ctx, s.Pos(), "assign to undefined name %q", s.Name)
		}
		if isConst {
			return rtErr(vm.ctx, s.Pos(), "assign to const name %q", s.Name)
		}
		return nil
	case *ReturnStmt:
		if s.Val == nil {
			return ret{v: Null()}
		}
		v, err := vm.eval(env, s.Val)
		if err != nil {
			return err
		}
		return ret{v: v}
	case *ExprStmt:
		_, err := vm.eval(env, s.Exp)
		return err
	case *FnDef:
		if env.HasLocal(s.Name) {
			return rtErr(vm.ctx, s.Pos(), "name already defined: %q", s.Name)
		}
		fn := &Func{
			Name: s.Name,
			Args: append([]string(nil), s.Params...),
			Body: s.Body,
			Env:  env,
			Pos:  s.Pos(),
		}
		v := Fn(fn)
		env.DefConst(s.Name, v)
		if s.Exported && exp != nil {
			exp[s.Name] = v
		}
		return nil
	case *IfStmt:
		c, err := vm.eval(env, s.Cond)
		if err != nil {
			return err
		}
		if c.IsTruthy() {
			return vm.execBlock(env, exp, s.Then)
		}
		for _, el := range s.Elifs {
			c2, err := vm.eval(env, el.Cond)
			if err != nil {
				return err
			}
			if c2.IsTruthy() {
				return vm.execBlock(env, exp, el.Body)
			}
		}
		if s.Else != nil {
			return vm.execBlock(env, exp, s.Else)
		}
		return nil
	case *ForStmt:
		return vm.execFor(env, exp, s)
	case *BreakStmt:
		return breakSig{pos: s.Pos()}
	case *ContinueStmt:
		return continueSig{pos: s.Pos()}
	default:
		return rtErr(vm.ctx, st.Pos(), "unknown stmt")
	}
}

func (vm *VM) execBlock(up *Env, exp map[string]Value, b *Block) error {
	if b == nil {
		return nil
	}
	env := NewEnv(up)
	for _, st := range b.Stmts {
		if err := vm.execStmt(env, exp, st); err != nil {
			if _, ok := err.(ret); ok {
				return err
			}
			return err
		}
	}
	return nil
}

func (vm *VM) execFor(up *Env, exp map[string]Value, s *ForStmt) error {
	if s.Range != nil {
		return vm.execRange(up, exp, s)
	}
	env := NewEnv(up)
	if s.Init != nil {
		if err := vm.execStmt(env, exp, s.Init); err != nil {
			return err
		}
	}
	for {
		if err := vm.tick(s.Pos()); err != nil {
			return err
		}
		if s.Cond != nil {
			c, err := vm.eval(env, s.Cond)
			if err != nil {
				return err
			}
			if !c.IsTruthy() {
				break
			}
		}
		err := vm.execBlock(env, exp, s.Body)
		if err != nil {
			switch err.(type) {
			case ret:
				return err
			case breakSig:
				return nil
			case continueSig:
				if s.Post != nil {
					if err := vm.execStmt(env, exp, s.Post); err != nil {
						return err
					}
				}
				continue
			default:
				return err
			}
		}
		if s.Post != nil {
			if err := vm.execStmt(env, exp, s.Post); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vm *VM) execRange(up *Env, exp map[string]Value, s *ForStmt) error {
	rng := s.Range
	if rng == nil {
		return nil
	}
	env := NewEnv(up)
	src, err := vm.eval(env, rng.Expr)
	if err != nil {
		return err
	}
	if rng.Declare {
		vm.defRangeVar(env, rng.Key)
		vm.defRangeVar(env, rng.Val)
	}
	switch src.K {
	case VList:
		for i, it := range src.L {
			if err := vm.tick(s.Pos()); err != nil {
				return err
			}
			if err := vm.assignRangeVar(
				env,
				rng.Key,
				Num(float64(i)),
				rng.Declare,
				s.Pos(),
			); err != nil {
				return err
			}
			if err := vm.assignRangeVar(env, rng.Val, it, rng.Declare, s.Pos()); err != nil {
				return err
			}
			if err := vm.execBlock(env, exp, s.Body); err != nil {
				switch err.(type) {
				case ret:
					return err
				case breakSig:
					return nil
				case continueSig:
					continue
				default:
					return err
				}
			}
		}
	case VDict:
		keys := make([]string, 0, len(src.M))
		for k := range src.M {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := vm.tick(s.Pos()); err != nil {
				return err
			}
			if err := vm.assignRangeVar(env, rng.Key, Str(k), rng.Declare, s.Pos()); err != nil {
				return err
			}
			if err := vm.assignRangeVar(env, rng.Val, src.M[k], rng.Declare, s.Pos()); err != nil {
				return err
			}
			if err := vm.execBlock(env, exp, s.Body); err != nil {
				switch err.(type) {
				case ret:
					return err
				case breakSig:
					return nil
				case continueSig:
					continue
				default:
					return err
				}
			}
		}
	case VStr:
		for idx, r := range src.S {
			if err := vm.tick(s.Pos()); err != nil {
				return err
			}
			if err := vm.assignRangeVar(
				env,
				rng.Key,
				Num(float64(idx)),
				rng.Declare,
				s.Pos(),
			); err != nil {
				return err
			}
			if err := vm.assignRangeVar(
				env,
				rng.Val,
				Str(string(r)),
				rng.Declare,
				s.Pos(),
			); err != nil {
				return err
			}
			if err := vm.execBlock(env, exp, s.Body); err != nil {
				switch err.(type) {
				case ret:
					return err
				case breakSig:
					return nil
				case continueSig:
					continue
				default:
					return err
				}
			}
		}
	default:
		return rtErr(vm.ctx, rng.Expr.Pos(), "range over non-iterable value")
	}
	return nil
}

func (vm *VM) defRangeVar(env *Env, name string) {
	if name == "" || name == "_" {
		return
	}
	env.DefConst(name, Null())
}

func (vm *VM) assignRangeVar(env *Env, name string, val Value, declare bool, pos Pos) error {
	if name == "" || name == "_" {
		return nil
	}
	if declare {
		env.DefConst(name, val)
		return nil
	}
	found, isConst := env.Set(name, val)
	if !found {
		return rtErr(vm.ctx, pos, "assign to undefined name %q", name)
	}
	if isConst {
		return rtErr(vm.ctx, pos, "assign to const name %q", name)
	}
	return nil
}

func (vm *VM) eval(env *Env, ex Expr) (Value, error) {
	if err := vm.tick(ex.Pos()); err != nil {
		return Null(), err
	}
	switch e := ex.(type) {
	case *Ident:
		v, ok := env.Get(e.Name)
		if !ok {
			return Null(), rtErr(vm.ctx, e.Pos(), "undefined name %q", e.Name)
		}
		return v, nil
	case *Literal:
		switch e.Kind {
		case LitNull:
			return Null(), nil
		case LitBool:
			return Bool(e.B), nil
		case LitNum:
			return Num(e.N), nil
		case LitStr:
			if err := vm.chkStr(e.Pos(), e.S); err != nil {
				return Null(), err
			}
			return Str(e.S), nil
		}
		return Null(), rtErr(vm.ctx, e.Pos(), "bad literal")
	case *Unary:
		x, err := vm.eval(env, e.X)
		if err != nil {
			return Null(), err
		}
		switch e.Op {
		case UnNot:
			return Bool(!x.IsTruthy()), nil
		case UnNeg:
			n, err := toNum(e.Pos(), x)
			if err != nil {
				return Null(), wrapErr(vm.ctx, err)
			}
			return Num(-n), nil
		}
		return Null(), rtErr(vm.ctx, e.Pos(), "bad unary")
	case *Binary:
		return vm.evalBin(env, e)
	case *Ternary:
		c, err := vm.eval(env, e.Cond)
		if err != nil {
			return Null(), err
		}
		if c.IsTruthy() {
			return vm.eval(env, e.Then)
		}
		return vm.eval(env, e.Else)
	case *TryExpr:
		v, err := vm.eval(env, e.X)
		if err != nil {
			if isAbort(err) {
				return Null(), err
			}
			return newResult(false, Null(), err), nil
		}
		return newResult(true, v, nil), nil
	case *Call:
		return vm.evalCall(env, e)
	case *Index:
		x, err := vm.eval(env, e.X)
		if err != nil {
			return Null(), err
		}
		idx, err := vm.eval(env, e.Idx)
		if err != nil {
			return Null(), err
		}
		switch x.K {
		case VList:
			if idx.K != VNum {
				return Null(), rtErr(vm.ctx, e.Pos(), "list index must be number")
			}
			i := int(idx.N)
			if i < 0 || i >= len(x.L) {
				return Null(), nil
			}
			return x.L[i], nil
		case VDict:
			k, err := toKey(e.Pos(), idx)
			if err != nil {
				return Null(), wrapErr(vm.ctx, err)
			}
			v, ok := x.M[k]
			if !ok {
				return Null(), nil
			}
			return v, nil
		case VObj:
			if x.O == nil {
				return Null(), rtErr(vm.ctx, e.Pos(), "object has no index")
			}
			v, err := x.O.Index(idx)
			if err != nil {
				return Null(), wrapErr(vm.ctx, err)
			}
			return v, nil
		default:
			return Null(), rtErr(vm.ctx, e.Pos(), "index on non-collection")
		}
	case *Member:
		x, err := vm.eval(env, e.X)
		if err != nil {
			return Null(), err
		}
		switch x.K {
		case VDict:
			v, ok := x.M[e.Name]
			if !ok {
				return Null(), nil
			}
			return v, nil
		case VObj:
			if x.O == nil {
				return Null(), rtErr(vm.ctx, e.Pos(), "object has no members")
			}
			v, ok := x.O.GetMember(e.Name)
			if !ok {
				return Null(), nil
			}
			return v, nil
		default:
			return Null(), rtErr(vm.ctx, e.Pos(), "member on non-object")
		}
	case *ListLit:
		if vm.ctx != nil && vm.ctx.Lim.MaxList > 0 && len(e.Elems) > vm.ctx.Lim.MaxList {
			return Null(), rtErr(vm.ctx, e.Pos(), "list too large")
		}
		out := make([]Value, 0, len(e.Elems))
		for _, it := range e.Elems {
			v, err := vm.eval(env, it)
			if err != nil {
				return Null(), err
			}
			out = append(out, v)
		}
		return List(out), nil
	case *DictLit:
		if vm.ctx != nil && vm.ctx.Lim.MaxDict > 0 && len(e.Entries) > vm.ctx.Lim.MaxDict {
			return Null(), rtErr(vm.ctx, e.Pos(), "dict too large")
		}
		out := make(map[string]Value, len(e.Entries))
		for _, it := range e.Entries {
			v, err := vm.eval(env, it.Val)
			if err != nil {
				return Null(), err
			}
			out[it.Key] = v
		}
		return Dict(out), nil
	default:
		return Null(), rtErr(vm.ctx, ex.Pos(), "unknown expr")
	}
}

func (vm *VM) evalCall(env *Env, c *Call) (Value, error) {
	cal, err := vm.eval(env, c.Callee)
	if err != nil {
		return Null(), err
	}
	args := make([]Value, 0, len(c.Args))
	for _, a := range c.Args {
		v, err := vm.eval(env, a)
		if err != nil {
			return Null(), err
		}
		args = append(args, v)
	}
	return vm.callVal(c.Pos(), cal, args)
}

func (vm *VM) callVal(pos Pos, cal Value, args []Value) (Value, error) {
	switch cal.K {
	case VFunc:
		fn := cal.F
		if fn == nil {
			return Null(), rtErr(vm.ctx, pos, "call on nil fn")
		}
		if len(args) != len(fn.Args) {
			return Null(), rtErr(vm.ctx, pos, "arg count mismatch")
		}
		if vm.ctx != nil {
			if vm.ctx.Lim.MaxCall > 0 && vm.ctx.depth >= vm.ctx.Lim.MaxCall {
				return Null(), rtErr(vm.ctx, pos, "call depth exceeded")
			}
			vm.ctx.depth++
			vm.ctx.push(Frame{Kind: FrameFn, Pos: fn.Pos, Name: fn.Name})
			defer func() {
				vm.ctx.pop()
				vm.ctx.depth--
			}()
		}
		env := NewEnv(fn.Env)
		for i, name := range fn.Args {
			env.Def(name, args[i])
		}
		err := vm.execBlock(env, nil, fn.Body)
		if err != nil {
			if r, ok := err.(ret); ok {
				return r.v, nil
			}
			if b, ok := err.(breakSig); ok {
				return Null(), rtErr(vm.ctx, b.pos, "break outside loop")
			}
			if c, ok := err.(continueSig); ok {
				return Null(), rtErr(vm.ctx, c.pos, "continue outside loop")
			}
			return Null(), err
		}
		return Null(), nil
	case VNative:
		if cal.NF == nil {
			return Null(), rtErr(vm.ctx, pos, "call on nil native")
		}
		v, err := cal.NF(vm.ctx, pos, args)
		if err != nil {
			return Null(), err
		}
		if err := vm.chkVal(pos, v); err != nil {
			return Null(), err
		}
		return v, nil
	default:
		return Null(), rtErr(vm.ctx, pos, "not callable")
	}
}

func (vm *VM) evalBin(env *Env, e *Binary) (Value, error) {
	switch e.Op {
	case OpAnd:
		l, err := vm.eval(env, e.Left)
		if err != nil {
			return Null(), err
		}
		if !l.IsTruthy() {
			return Bool(false), nil
		}
		r, err := vm.eval(env, e.Right)
		if err != nil {
			return Null(), err
		}
		return Bool(r.IsTruthy()), nil
	case OpOr:
		l, err := vm.eval(env, e.Left)
		if err != nil {
			return Null(), err
		}
		if l.IsTruthy() {
			return Bool(true), nil
		}
		r, err := vm.eval(env, e.Right)
		if err != nil {
			return Null(), err
		}
		return Bool(r.IsTruthy()), nil
	case OpCoalesce:
		l, err := vm.eval(env, e.Left)
		if err != nil {
			return Null(), err
		}
		if l.K != VNull {
			return l, nil
		}
		return vm.eval(env, e.Right)
	}

	l, err := vm.eval(env, e.Left)
	if err != nil {
		return Null(), err
	}
	r, err := vm.eval(env, e.Right)
	if err != nil {
		return Null(), err
	}

	switch e.Op {
	case OpAdd:
		if l.K == VNum && r.K == VNum {
			return Num(l.N + r.N), nil
		}
		ls, err := toStr(vm.ctx, e.Pos(), l)
		if err != nil {
			return Null(), err
		}
		rs, err := toStr(vm.ctx, e.Pos(), r)
		if err != nil {
			return Null(), err
		}
		if err := vm.chkStr(e.Pos(), ls+rs); err != nil {
			return Null(), err
		}
		return Str(ls + rs), nil
	case OpSub, OpMul, OpDiv, OpMod:
		ln, err := toNum(e.Pos(), l)
		if err != nil {
			return Null(), wrapErr(vm.ctx, err)
		}
		rn, err := toNum(e.Pos(), r)
		if err != nil {
			return Null(), wrapErr(vm.ctx, err)
		}
		switch e.Op {
		case OpSub:
			return Num(ln - rn), nil
		case OpMul:
			return Num(ln * rn), nil
		case OpDiv:
			if rn == 0 {
				return Null(), rtErr(vm.ctx, e.Pos(), "division by zero")
			}
			return Num(ln / rn), nil
		case OpMod:
			if rn == 0 {
				return Null(), rtErr(vm.ctx, e.Pos(), "division by zero")
			}
			return Num(math.Mod(ln, rn)), nil
		}
	case OpEq:
		return Bool(eq(l, r)), nil
	case OpNe:
		return Bool(!eq(l, r)), nil
	case OpLt, OpLe, OpGt, OpGe:
		return cmp(vm.ctx, e.Pos(), e.Op, l, r)
	}

	return Null(), rtErr(vm.ctx, e.Pos(), "bad op")
}

func (vm *VM) tick(pos Pos) error {
	if vm.ctx == nil {
		return nil
	}
	return vm.ctx.tick(pos)
}

func (vm *VM) chkVal(pos Pos, v Value) error {
	if v.K == VStr {
		return vm.chkStr(pos, v.S)
	}
	if vm.ctx == nil {
		return nil
	}
	switch v.K {
	case VList:
		if vm.ctx.Lim.MaxList > 0 && len(v.L) > vm.ctx.Lim.MaxList {
			return rtErr(vm.ctx, pos, "list too large")
		}
	case VDict:
		if vm.ctx.Lim.MaxDict > 0 && len(v.M) > vm.ctx.Lim.MaxDict {
			return rtErr(vm.ctx, pos, "dict too large")
		}
	}
	return nil
}

func (vm *VM) chkStr(pos Pos, s string) error {
	if vm.ctx == nil || vm.ctx.Lim.MaxStr <= 0 {
		return nil
	}
	if len(s) > vm.ctx.Lim.MaxStr {
		return rtErr(vm.ctx, pos, "string too long")
	}
	return nil
}

func eq(a, b Value) bool {
	if a.K != b.K {
		return false
	}
	switch a.K {
	case VNull:
		return true
	case VBool:
		return a.B == b.B
	case VNum:
		return a.N == b.N
	case VStr:
		return a.S == b.S
	default:
		return false
	}
}

func cmp(ctx *Ctx, pos Pos, op BinOp, a, b Value) (Value, error) {
	if a.K == VNum && b.K == VNum {
		switch op {
		case OpLt:
			return Bool(a.N < b.N), nil
		case OpLe:
			return Bool(a.N <= b.N), nil
		case OpGt:
			return Bool(a.N > b.N), nil
		case OpGe:
			return Bool(a.N >= b.N), nil
		}
	}
	if a.K == VStr && b.K == VStr {
		switch op {
		case OpLt:
			return Bool(a.S < b.S), nil
		case OpLe:
			return Bool(a.S <= b.S), nil
		case OpGt:
			return Bool(a.S > b.S), nil
		case OpGe:
			return Bool(a.S >= b.S), nil
		}
	}
	return Null(), rtErr(ctx, pos, "cannot compare")
}
