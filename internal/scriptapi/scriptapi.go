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

var objects = map[string][]row{
	"vars": {
		{both, Member{Name: "get", Args: "name", Summary: "Read a variable"}},
		{both, Member{Name: "has", Args: "name", Summary: "Check whether a variable exists"}},
		{rts, Member{
			Name:    "require",
			Args:    "name",
			Summary: "Read a variable or fail when it is empty, with an optional message",
		}},
		{both, Member{Name: "set", Args: "name, value", Summary: "Set a variable for this request", Writes: true}},
		{both, Member{Name: "interpolate", Args: "text", Summary: "Fill {{name}} placeholders in text"}},
		{both, Member{Name: "global", Summary: "Global variables shared across requests"}},
	},
	"vars.global": {
		{both, Member{Name: "get", Args: "name", Summary: "Read a global variable"}},
		{both, Member{Name: "has", Args: "name", Summary: "Check whether a global variable exists"}},
		{rts, Member{
			Name:    "require",
			Args:    "name",
			Summary: "Read a global variable or fail when it is empty, with an optional message",
		}},
		{both, Member{
			Name:    "set",
			Args:    "name, value",
			Summary: "Set a global variable, with an optional secret flag",
			Writes:  true,
		}},
		{both, Member{Name: "delete", Args: "name", Summary: "Delete a global variable", Writes: true}},
	},
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
