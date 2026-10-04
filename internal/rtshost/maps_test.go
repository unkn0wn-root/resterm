package rtshost

import (
	"maps"
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/scriptapi"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestVarsMembersMatchScriptAPI(t *testing.T) {
	for object, o := range map[string]*mapObj{
		"vars":        newVarsObj(Scope{}, nil, nil),
		"vars.global": newGlobalObj(vars.NameView[string]{}, nil),
	} {
		var want []string
		for _, m := range scriptapi.Members(object, restfile.ScriptLangRTS) {
			want = append(want, m.Name)
		}
		slices.Sort(want)
		if got := slices.Sorted(maps.Keys(o.members)); !slices.Equal(got, want) {
			t.Errorf("%s members = %q, scriptapi lists %q", object, got, want)
		}
	}
}
