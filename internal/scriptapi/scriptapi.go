package scriptapi

import "github.com/unkn0wn-root/resterm/internal/restfile"

type langSet uint8

const (
	js langSet = 1 << iota
	rts
	both = js | rts
)

type Member struct {
	Name string
	// Args holds required argument names, such as "name, value".
	Args    string
	Summary string
	// Object is true for nested objects such as vars.global.
	Object bool
	// Writes is true for methods that change variables.
	Writes bool
}

type row struct {
	langs langSet
	Member
}

// TestMembersMatchScriptAPI in internal/scripts and internal/rtshost checks
// these tables against both runtimes. Add new objects to both tests.
var objects = map[string][]row{
	"vars": append(mapRows("variable"),
		row{both, Member{Name: "set", Args: "name, value", Summary: "Set a variable for this request", Writes: true}},
		row{both, Member{Name: "interpolate", Args: "text", Summary: "Fill {{name}} placeholders in text"}},
		row{both, Member{Name: "global", Summary: "Global variables shared across requests"}},
	),
	"vars.global": append(mapRows("global variable"),
		row{both, Member{
			Name:    "set",
			Args:    "name, value",
			Summary: "Set a global variable, with an optional secret flag",
			Writes:  true,
		}},
		row{both, Member{Name: "delete", Args: "name", Summary: "Delete a global variable", Writes: true}},
	),
}

// mapRows lists the lookup methods that every RTS mapObj has.
func mapRows(noun string) []row {
	return []row{
		{both, Member{Name: "get", Args: "name", Summary: "Read a " + noun}},
		{both, Member{Name: "has", Args: "name", Summary: "Check whether a " + noun + " exists"}},
		{rts, Member{
			Name:    "require",
			Args:    "name",
			Summary: "Read a " + noun + " or fail when it is empty, with an optional message",
		}},
	}
}

// Members returns an object's members for the given script language.
// lang is restfile.ScriptLangJS or restfile.ScriptLangRTS.
func Members(object, lang string) []Member {
	var want langSet
	switch lang {
	case restfile.ScriptLangJS:
		want = js
	case restfile.ScriptLangRTS:
		want = rts
	}
	var out []Member
	for _, r := range objects[object] {
		if r.langs&want == 0 {
			continue
		}
		m := r.Member
		_, m.Object = objects[object+"."+m.Name]
		out = append(out, m)
	}
	return out
}
