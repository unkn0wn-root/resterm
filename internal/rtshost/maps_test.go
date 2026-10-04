package rtshost

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/rts/native"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestMembersMatchScriptAPI(t *testing.T) {
	v := newVarsObj(Scope{}, nil, nil)
	g := newGlobalObj(vars.NameView[string]{}, nil)
	for _, tt := range []struct {
		obj  *mapObj
		defs []native.Def
	}{
		{v, []native.Def{
			v.getDef(), v.hasDef(), v.requireDef(), varsSetDef(v, nil), varsInterpolateDef(v.value),
		}},
		{g, []native.Def{
			g.getDef(), g.hasDef(), g.requireDef(), globalSetDef(g, nil), globalDeleteDef(g, nil),
		}},
	} {
		sigs := map[string]string{}
		for _, d := range tt.defs {
			sigs[d.Name()] = d.Sig()
		}
		var want []string
		for _, m := range scriptapi.Members(tt.obj.name, restfile.ScriptLangRTS) {
			want = append(want, m.Name)
			if m.Object {
				continue
			}
			// Args lists required arguments only, so drop the optional [...] part.
			name := tt.obj.name + "." + m.Name
			req, _, _ := strings.Cut(sigs[name], "[")
			if strings.TrimSuffix(req, ")") != name+"("+m.Args {
				t.Errorf("%s sig = %q, scriptapi args %q", name, sigs[name], m.Args)
			}
		}
		slices.Sort(want)
		if got := slices.Sorted(maps.Keys(tt.obj.members)); !slices.Equal(got, want) {
			t.Errorf("%s members = %q, scriptapi lists %q", tt.obj.name, got, want)
		}
	}
}
