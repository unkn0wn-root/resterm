package stdlib

import (
	"maps"

	"github.com/unkn0wn-root/resterm/internal/rts"
)

type nsSpec struct {
	// name is the RTS namespace name, such as "crypto" or "json".
	name string
	// top exposes the namespace both as rts.name and as a top-level alias.
	top bool
	fns map[string]rts.NativeFunc
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

type objMap struct {
	name string
	m    map[string]rts.Value
}

func (o *objMap) TypeName() string { return o.name }

func (o *objMap) GetMember(name string) (rts.Value, bool) {
	v, ok := o.m[name]
	return v, ok
}

func (o *objMap) Index(key rts.Value) (rts.Value, error) {
	k, err := rts.Key(rts.Pos{}, key)
	if err != nil {
		return rts.Null(), err
	}

	v, ok := o.m[k]
	if !ok {
		return rts.Null(), nil
	}
	return v, nil
}

func addVals(dst, src map[string]rts.Value) {
	maps.Copy(dst, src)
}

func mkFns(prefix string, fns map[string]rts.NativeFunc) map[string]rts.Value {
	out := make(map[string]rts.Value, len(fns))
	for k, f := range fns {
		name := k
		if prefix != "" {
			name = prefix + "." + k
		}
		out[k] = rts.NativeNamed(name, f)
	}
	return out
}

func mkObj(name string, fns map[string]rts.NativeFunc) *objMap {
	return &objMap{name: name, m: mkFns(name, fns)}
}

// New builds a fresh RTS standard-library prelude.
func New() map[string]rts.Value {
	core := mkFns("", coreSpec)
	specs := rtsNamespaces
	rootExtra := 3

	top := 0
	for _, s := range specs {
		if s.top {
			top++
		}
	}

	out := make(map[string]rts.Value, len(core)+top+rootExtra)
	addVals(out, core)

	rootMembers := make(map[string]rts.Value, len(core)+len(specs)+1)
	addVals(rootMembers, core)
	for _, s := range specs {
		o := mkObj(s.name, s.fns)
		if s.top {
			out[s.name] = rts.Obj(o)
		}
		rootMembers[s.name] = rts.Obj(o)
	}

	enc := mkEncObj()
	out["encoding"] = rts.Obj(enc)
	rootMembers["encoding"] = rts.Obj(enc)

	rtsRoot := &objMap{name: "rts", m: rootMembers}
	stdlibRoot := &objMap{name: "stdlib", m: rootMembers}
	out["rts"] = rts.Obj(rtsRoot)
	out["stdlib"] = rts.Obj(stdlibRoot)

	return out
}
