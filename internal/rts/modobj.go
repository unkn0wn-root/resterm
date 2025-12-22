package rts

import "fmt"

type ModObj struct {
	name string
	exp  map[string]Value
}

func NewModObj(name string, exp map[string]Value) *ModObj {
	cp := make(map[string]Value, len(exp))
	for k, v := range exp {
		cp[k] = v
	}
	return &ModObj{name: name, exp: cp}
}

func (m *ModObj) TypeName() string { return "module:" + m.name }

func (m *ModObj) GetMember(name string) (Value, bool) {
	v, ok := m.exp[name]
	return v, ok
}

func (m *ModObj) CallMember(name string, args []Value) (Value, error) {
	return Null(), fmt.Errorf("module has no member call: %s", name)
}

func (m *ModObj) Index(key Value) (Value, error) {
	k, err := toKey(Pos{}, key)
	if err != nil {
		return Null(), err
	}

	v, ok := m.exp[k]
	if !ok {
		return Null(), nil
	}
	return v, nil
}
