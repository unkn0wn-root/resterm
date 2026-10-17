package stdlib

import "github.com/unkn0wn-root/resterm/internal/rts"

type nsSpec struct {
	// name is the RTS namespace name, such as "crypto" or "json".
	name string
	fns  map[string]rts.NativeFunc
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

func (o *objMap) Member(_ *rts.Ctx, _ rts.Pos, name string) (rts.Value, bool, error) {
	v, ok := o.m[name]
	return v, ok, nil
}

func (o *objMap) Index(_ *rts.Ctx, pos rts.Pos, key rts.Value) (rts.Value, error) {
	k, err := rts.Key(pos, key)
	if err != nil {
		return rts.Null(), err
	}

	v, ok := o.m[k]
	if !ok {
		return rts.Null(), nil
	}
	return v, nil
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

// New returns the predeclared names of the standard library: the core helpers,
// such as len and str, and one object for each namespace, such as json.
func New() map[string]rts.Value {
	out := mkFns("", coreSpec)
	for _, s := range rtsNamespaces {
		out[s.name] = rts.Obj(mkObj(s.name, s.fns))
	}
	out["encoding"] = rts.Obj(mkEncObj())
	return out
}
