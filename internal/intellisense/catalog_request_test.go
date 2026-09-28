package intellisense

import (
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func optionLabels(keys []string) []string {
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = key + "="
	}
	return out
}

// Each state offers only what the parser accepts next.
func TestAuthArgsFollowTheGrammar(t *testing.T) {
	kinds := []string{"basic", "bearer", "apikey", "oauth2", "command"}
	request := slices.Concat([]string{"none"}, kinds, []string{"use="})
	command := optionLabels(restfile.AuthCommandParams)
	tests := []struct {
		line string
		want []string
	}{
		{"# @auth ", slices.Concat([]string{"request", "file", "global"}, request)},
		{"# @auth request ", request},
		{"# @auth file ", kinds},
		{"# @auth global ", kinds},
		{"# @auth command ", command},
		{"# @auth file command ", slices.Concat([]string{"name="}, command)},
		{"# @auth global command name=gh cmd=x ", slices.DeleteFunc(slices.Clone(command), func(l string) bool {
			return l == "cmd="
		})},
		{"# @auth use=gh ", optionLabels(restfile.AuthUseParams)},
		{"# @auth apikey ", []string{"header", "query"}},
		{"# @auth apikey header ", nil},
		{"# @auth basic ", nil},
		{"# @auth bearer ", nil},
		{"# @auth none ", nil},
	}
	for _, tt := range tests {
		if got := labels(suggest(tt.line, Scope{})); !slices.Equal(got, tt.want) {
			t.Errorf("%q\n got: %v\nwant: %v", tt.line, got, tt.want)
		}
	}
}

func TestAuthOAuth2OffersOnlyItsOptions(t *testing.T) {
	got := labels(suggest("# @auth file oauth2 ", Scope{}))
	for _, want := range []string{"token_url=", "client_id=", "grant=", "cache_key=", "header="} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	for _, bad := range []string{"cmd=", "scheme=", "timeout=", "name=", "use=", "basic"} {
		if slices.Contains(got, bad) {
			t.Errorf("offered %q, which oauth2 does not take: %v", bad, got)
		}
	}
}

// A request needs a command right away. A definition is usually named first.
func TestAuthCommandInsertDependsOnScope(t *testing.T) {
	req := suggest("# @auth com", Scope{})
	if len(req) != 1 || req[0].InsertText() != `command cmd="gh auth token"` || req[0].Placeholder != "gh auth token" {
		t.Fatalf("request command = %+v", req)
	}
	for _, line := range []string{"# @auth file com", "# @auth global com"} {
		def := suggest(line, Scope{})
		if len(def) != 1 || def[0].InsertText() != "command" || def[0].Placeholder != "" || !def[0].Continue {
			t.Fatalf("%q: definition command = %+v", line, def)
		}
	}
}

func TestAuthValuesCompleteAfterTheKind(t *testing.T) {
	sc := Scope{Profiles: ProfileSet{Auth: []string{"gh"}}}
	tests := map[string][]string{
		"# @auth file command format=": {"text", "json"},
		"# @auth command format=j":     {"json"},
		"# @auth request use=":         {"gh"},
	}
	for line, want := range tests {
		if got := labels(suggest(line, sc)); !slices.Equal(got, want) {
			t.Errorf("%q = %v, want %v", line, got, want)
		}
	}
}

// Editing a word in place offers the words that could stand there.
func TestAuthCaretOnAWordUsesItsOwnState(t *testing.T) {
	line := "# @auth file command cmd=x"
	col := len("# @auth file comm")
	ctx, ok := Analyze(testLines{line}, 0, col)
	if !ok {
		t.Fatal("no context")
	}
	got := labels(directiveSource{}.Provide(ctx, Scope{}))
	if !slices.Equal(got, []string{"command"}) || ctx.End != len("# @auth file command") {
		t.Fatalf("items = %v, end = %d", got, ctx.End)
	}
}
