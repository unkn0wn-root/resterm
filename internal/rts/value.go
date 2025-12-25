package rts

type VKind int

const (
	VNull VKind = iota
	VBool
	VNum
	VStr
	VList
	VDict
	VFunc
	VNative
	VObj
)

type Value struct {
	K VKind

	B bool
	N float64
	S string
	L []Value
	M map[string]Value

	F  *Func
	NF NativeFunc
	O  Object
}

type Func struct {
	Name string
	Args []string
	Body *Block
	Env  *Env
	Pos  Pos
}

type Object interface {
	TypeName() string
	GetMember(name string) (Value, bool)
	CallMember(name string, args []Value) (Value, error)
	Index(key Value) (Value, error)
}

func Null() Value                   { return Value{K: VNull} }
func Bool(v bool) Value             { return Value{K: VBool, B: v} }
func Num(v float64) Value           { return Value{K: VNum, N: v} }
func Str(v string) Value            { return Value{K: VStr, S: v} }
func List(v []Value) Value          { return Value{K: VList, L: v} }
func Dict(v map[string]Value) Value { return Value{K: VDict, M: v} }
func Fn(v *Func) Value              { return Value{K: VFunc, F: v} }
func Native(v NativeFunc) Value     { return Value{K: VNative, NF: v} }
func Obj(v Object) Value            { return Value{K: VObj, O: v} }

func (v Value) IsTruthy() bool {
	switch v.K {
	case VNull:
		return false
	case VBool:
		return v.B
	case VNum:
		return v.N != 0
	case VStr:
		return v.S != ""
	case VList:
		return len(v.L) != 0
	case VDict:
		return len(v.M) != 0
	case VObj:
		if v.O != nil {
			if t, ok := v.O.(interface{ Truthy() bool }); ok {
				return t.Truthy()
			}
		}
		return true
	default:
		return true
	}
}

func ValueEqual(a, b Value) bool {
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

type Env struct {
	Up   *Env
	Vars map[string]Binding
}

type Binding struct {
	V     Value
	Const bool
}

func NewEnv(up *Env) *Env {
	return &Env{Up: up, Vars: map[string]Binding{}}
}

func (e *Env) Def(name string, v Value) {
	e.DefWith(name, v, false)
}

func (e *Env) DefConst(name string, v Value) {
	e.DefWith(name, v, true)
}

func (e *Env) DefWith(name string, v Value, isConst bool) {
	e.Vars[name] = Binding{V: v, Const: isConst}
}

func (e *Env) HasLocal(name string) bool {
	if e == nil {
		return false
	}
	_, ok := e.Vars[name]
	return ok
}

func (e *Env) Set(name string, v Value) (bool, bool) {
	for cur := e; cur != nil; cur = cur.Up {
		if b, ok := cur.Vars[name]; ok {
			if b.Const {
				return true, true
			}
			cur.Vars[name] = Binding{V: v, Const: b.Const}
			return true, false
		}
	}
	return false, false
}

func (e *Env) Get(name string) (Value, bool) {
	for cur := e; cur != nil; cur = cur.Up {
		if v, ok := cur.Vars[name]; ok {
			return v.V, true
		}
	}
	return Null(), false
}
