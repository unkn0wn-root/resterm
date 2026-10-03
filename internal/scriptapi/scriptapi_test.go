package scriptapi

import (
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestMembers(t *testing.T) {
	tests := []struct {
		object string
		lang   string
		want   []string
	}{
		{"vars", restfile.ScriptLangJS, []string{"get", "has", "set", "interpolate", "global"}},
		{"vars", restfile.ScriptLangRTS, []string{"get", "has", "require", "set", "interpolate", "global"}},
		{"vars.global", restfile.ScriptLangJS, []string{"get", "has", "set", "delete"}},
		{"vars.global", restfile.ScriptLangRTS, []string{"get", "has", "require", "set", "delete"}},
		{"vars", "python", nil},
		{"vars", "", nil},
		{"request", restfile.ScriptLangRTS, nil},
	}
	for _, tt := range tests {
		var got []string
		for _, m := range Members(tt.object, tt.lang) {
			got = append(got, m.Name)
			if m.Object != (m.Name == "global") {
				t.Errorf("%s.%s Object = %t", tt.object, m.Name, m.Object)
			}
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("Members(%q, %q) = %q, want %q", tt.object, tt.lang, got, tt.want)
		}
	}
}
