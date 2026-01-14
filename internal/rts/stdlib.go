package rts

import (
	"fmt"
	"maps"
)

type nsSpec struct {
	name string
	top  bool
	fns  map[string]NativeFunc
}

var rtsNamespaces = []nsSpec{
	cryptoSpec,
	base64Spec,
	urlSpec,
	timeSpec,
	jsonSpec,
	headersSpec,
	querySpec,
	textSpec,
	listSpec,
	dictSpec,
	mathSpec,
}

type NativeFunc func(ctx *Ctx, pos Pos, args []Value) (Value, error)

type objMap struct {
	name string
	m    map[string]Value
}

func (o *objMap) TypeName() string { return o.name }

func (o *objMap) GetMember(name string) (Value, bool) {
	v, ok := o.m[name]
	return v, ok
}

func (o *objMap) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("no such member: %s", name)
}

func (o *objMap) Index(key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}

	v, ok := o.m[k]
	if !ok {
		return Null(), nil
	}
	return v, nil
}

func NativeNamed(name string, f NativeFunc) Value {
	return Native(func(ctx *Ctx, pos Pos, args []Value) (Value, error) {
		if ctx != nil {
			ctx.push(Frame{Kind: FrameNative, Pos: pos, Name: name})
			defer ctx.pop()
		}
		return f(ctx, pos, args)
	})
}

func addVals(dst, src map[string]Value) {
	maps.Copy(dst, src)
}

func mkFns(prefix string, fns map[string]NativeFunc) map[string]Value {
	out := make(map[string]Value, len(fns))
	for k, f := range fns {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out[k] = NativeNamed(name, f)
	}
	return out
}

func mkObj(name string, fns map[string]NativeFunc) *objMap {
	return &objMap{name: name, m: mkFns(name, fns)}
}

func Stdlib() map[string]Value {
	return buildStdlib(nil)
}

func buildStdlib(req *requestObj) map[string]Value {
	core := mkFns("", coreSpec)
	specs := rtsNamespaces
	rootExtra := 3
	if req != nil {
		rootExtra++
	}

	top := 0
	for _, s := range specs {
		if s.top {
			top++
		}
	}

	out := make(map[string]Value, len(core)+top+rootExtra)
	addVals(out, core)

	rootMembers := make(map[string]Value, len(core)+len(specs)+1)
	addVals(rootMembers, core)
	for _, s := range specs {
		o := mkObj(s.name, s.fns)
		if s.top {
			out[s.name] = Obj(o)
		}
		rootMembers[s.name] = Obj(o)
	}

	enc := mkEncObj()
	out["encoding"] = Obj(enc)
	rootMembers["encoding"] = Obj(enc)

	rtsRoot := &objMap{name: "rts", m: rootMembers}
	stdlibRoot := &objMap{name: "stdlib", m: rootMembers}
	out["rts"] = Obj(rtsRoot)
	out["stdlib"] = Obj(stdlibRoot)

	if req != nil {
		out["request"] = Obj(req)
	}
	return out
}
