package intellisense

import (
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

func analyzeLine(t *testing.T, line string) Context {
	t.Helper()
	ctx, ok := Analyze(testLines{line}, 0, len([]rune(line)))
	if !ok {
		t.Fatalf("Analyze(%q) returned no context", line)
	}
	return ctx
}

func labels(items []Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.Label
	}
	return out
}

func TestAnalyzeArgumentValues(t *testing.T) {
	tests := []struct {
		line  string
		key   string
		query string
		kind  valueKind
		span  [2]int
	}{
		{
			line:  "# @settings http-insecure=Tr",
			key:   "http-insecure",
			query: "Tr",
			kind:  valueChoice,
			span:  [2]int{26, 28},
		},
		{
			line:  "# @setting http-version 2",
			key:   "http-version",
			query: "2",
			kind:  valueChoice,
			span:  [2]int{24, 25},
		},
		{
			line:  "# @settings http-root-ca=certs/ø",
			key:   "http-root-cas",
			query: "certs/ø",
			kind:  valuePath,
			span:  [2]int{25, 32},
		},
		{
			line:  `# @compare "Dév One" base="Dé`,
			key:   "base",
			query: "Dé",
			kind:  valueNames,
			span:  [2]int{26, 29},
		},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			ctx := analyzeLine(t, tt.line)
			if ctx.arg == nil {
				t.Fatalf("context completes no value: %+v", ctx)
			}
			if got := argKey(ctx); got != tt.key {
				t.Fatalf("argument = %q, want %q", got, tt.key)
			}
			if ctx.Query != tt.query {
				t.Fatalf("query = %q, want %q", ctx.Query, tt.query)
			}
			if ctx.arg.value.kind != tt.kind {
				t.Fatalf("value kind = %d, want %d", ctx.arg.value.kind, tt.kind)
			}
			if got := [2]int{ctx.Start, ctx.End}; got != tt.span {
				t.Fatalf("replacement span = %v, want %v", got, tt.span)
			}
		})
	}
}

func TestArgumentValueSuggestions(t *testing.T) {
	tests := []struct {
		line string
		want []string
	}{
		{"# @settings http-insecure=", []string{"true", "false"}},
		{"# @settings http-version=", []string{"1.1", "2"}},
		{"# @body expand ", []string{"true", "false"}},
		{"# @graphql ", []string{"true", "false"}},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := labels(suggest(tt.line, Scope{}))
			if !slices.Equal(got, tt.want) {
				t.Fatalf("values = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSingleSettingLineEndsAtItsValue(t *testing.T) {
	for _, line := range []string{
		"# @setting http-version 2 ",
		"# @setting insecure=true ",
		"# @body expand true ",
		"# @graphql true ",
	} {
		if items := suggest(line, Scope{}); len(items) != 0 {
			t.Fatalf("%q carries one setting yet still offers %q", line, labels(items))
		}
	}
	if items := suggest("# @settings insecure=true ", Scope{}); len(items) == 0 {
		t.Fatal("@settings should keep offering the options it has not been given")
	}
}

func TestCompletedArgumentsAndAliasesAreFiltered(t *testing.T) {
	items := suggest("# @settings insecure=true ", Scope{})
	if slices.Contains(labels(items), "insecure=") {
		t.Fatalf("completed option was offered again: %v", items)
	}

	items = suggest("# @compare dev prod baseline=dev ", Scope{Environments: []string{"dev", "prod", "qa"}})
	for _, alias := range []string{"base=", "baseline=", "primary=", "ref="} {
		if slices.Contains(labels(items), alias) {
			t.Fatalf("completed alias family %q was offered: %v", alias, items)
		}
	}
}

func TestCompareSuggestionsRespectSelectedTargets(t *testing.T) {
	flat := Scope{Environments: []string{"dev", "prod", "qa"}}
	for _, line := range []string{"# @compare base=", "# @compare baseline="} {
		if got := labels(suggest(line, flat)); !slices.Equal(got, flat.Environments) {
			t.Fatalf("%q baseline choices = %q, want %q", line, got, flat.Environments)
		}
	}
	if got := labels(suggest("# @compare dev prod base=", flat)); !slices.Equal(got, []string{"dev", "prod"}) {
		t.Fatalf("baseline choices = %q, want only selected targets", got)
	}

	scope := Scope{
		EnvironmentGroups: map[string][]string{
			"api west": {"dev west", "prod west"},
			"app":      {"dev app"},
		},
	}

	got := labels(suggest(`# @compare group="api west" `, scope))
	if !slices.Contains(got, "dev west") || !slices.Contains(got, "prod west") || slices.Contains(got, "dev app") {
		t.Fatalf("grouped compare choices = %q", got)
	}
	for _, label := range got {
		if strings.HasPrefix(label, "group=") {
			t.Errorf("selected group still offers %q", label)
		}
	}

	got = labels(suggest(`# @compare "dev west" "prod west" base=`, scope))
	if !slices.Equal(got, []string{"dev west", "prod west"}) {
		t.Fatalf("baseline choices = %q", got)
	}
}

func TestDynamicValuesRoundTripThroughTheParser(t *testing.T) {
	scope := Scope{
		RequestNames:      []string{"Create User"},
		EnvironmentGroups: map[string][]string{"api west": {"dev west"}},
		Profiles:          ProfileSet{Patch: []string{"json api"}},
	}
	lines := []string{
		"# @step one using=",
		"# @step one run=",
		"# @compare group=",
		"# @compare base=",
		"# @apply use=",
	}
	for _, line := range lines {
		items := suggest(line, scope)
		if len(items) != 1 {
			t.Fatalf("%q suggestions = %v, want one scoped name", line, items)
		}
		for _, item := range items {
			if strings.ContainsAny(item.Label, `"\`) {
				t.Fatalf("%q offered a quoted label %q", line, item.Label)
			}
			fields := directive.Fields(item.InsertText())
			if len(fields) != 1 || fields[0] != item.Label {
				t.Fatalf("%q inserts %q, which reads back as %q", line, item.InsertText(), fields)
			}
		}
	}
}

func TestAnalyzeFilesystemContexts(t *testing.T) {
	tests := []struct {
		line      string
		kind      PathKind
		quote     bool
		continues bool
		home      bool
		list      bool
	}{
		{line: "# @ssh key=~/.ssh/id", kind: PathAny, quote: true, continues: true, home: true},
		{line: "# @k8s kubeconfig=~/.kube/config", kind: PathAny, quote: true, continues: true, home: true},
		{line: "# @settings grpc-client-cert=certs/client.pem", kind: PathAny, quote: true, continues: true},
		{line: "# @settings http-root-cas=a.pem,b", kind: PathAny, quote: true, continues: true, list: true},
		{line: "# @setting http-client-key certs/k", kind: PathAny, quote: true},
		{line: "# @use ./rts/help", kind: PathRTS, quote: true, continues: true},
		{line: "# @grpc-descriptor ./proto/api", kind: PathAny},
		{line: "# @ws send-file < fixtures/data", kind: PathAny},
		{line: "# @query < queries/main", kind: PathGraphQL},
		{line: "# @variables < data/vars", kind: PathJSON},
		{line: "< payloads/body", kind: PathAny},
		{line: "> < scripts/pre", kind: PathScript},
		{line: "@ fixtures/part", kind: PathAny},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			ctx := analyzeLine(t, tt.line)
			want := PathContext{
				Kind:      tt.kind,
				Quote:     tt.quote,
				List:      tt.list,
				Home:      tt.home,
				Continues: tt.continues,
			}
			if ctx.Kind != KindPath || ctx.Path != want {
				t.Fatalf("path context = %v %+v, want %+v", ctx.Kind, ctx.Path, want)
			}
			if ctx.Query == "" || strings.HasPrefix(ctx.Query, " ") {
				t.Fatalf("path query = %q", ctx.Query)
			}
		})
	}
}

func TestInlineArgumentsAreNotPaths(t *testing.T) {
	for _, line := range []string{"# @query { user", "# @variables {\"id\""} {
		if ctx := analyzeLine(t, line); ctx.Kind == KindPath {
			t.Fatalf("%q was read as a path: %+v", line, ctx)
		}
	}
	items := suggest("# @query ", Scope{})
	if !slices.Contains(labels(items), "<") {
		t.Fatalf("the file marker is missing from %v", items)
	}
}

func TestVariableClosersAndHelperUsage(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"{{ho", "host}}"},
		{"{{ho}", "host}"},
		{"{{ho}}", "host"},
	}
	for _, tt := range tests {
		ctx, ok := Analyze(testLines{tt.line}, 0, 4)
		if !ok {
			t.Fatalf("Analyze(%q) returned no context", tt.line)
		}
		items := variableSource{}.Provide(ctx, Scope{Variables: []VarRef{{Name: "host"}}})
		if len(items) != 1 || items[0].InsertText() != tt.want {
			t.Fatalf("%q insert = %#v, want %q", tt.line, items, tt.want)
		}
	}

	ctx, _ := Analyze(testLines{"{{$randomI"}, 0, len([]rune("{{$randomI")))
	items := variableSource{}.Provide(ctx, Scope{})
	for _, item := range items {
		if item.Label != "$randomInt" {
			continue
		}
		if item.InsertText() != "$randomInt(1, 100)}}" || item.Placeholder != "1, 100" {
			t.Fatalf("helper item = %+v", item)
		}
		return
	}
	t.Fatal("$randomInt completion missing")
}

func TestCompletedTraceBudgetsAreFiltered(t *testing.T) {
	for _, line := range []string{
		"# @trace dns<=50ms total<=400ms ",
		"# @trace dns<=50ms total=400ms ",
	} {
		items := labels(suggest(line, Scope{}))
		for _, label := range []string{"dns<=", "total<=", "total="} {
			if slices.Contains(items, label) {
				t.Errorf("%q repeats %q: %v", line, label, items)
			}
		}
		if !slices.Contains(items, "connect<=") {
			t.Errorf("%q lost the remaining budgets: %v", line, items)
		}
	}
	items := suggest("# @trace dns<=", Scope{})
	if len(items) != 1 || items[0].InsertText() != "50ms" || items[0].Placeholder != "50ms" {
		t.Fatalf("budget values = %+v", items)
	}
}

func TestDirectiveArgumentContainingAtSign(t *testing.T) {
	items := labels(suggest("# @auth oauth2 username=person@example.com grant=", Scope{}))
	if !slices.Contains(items, "password") {
		t.Fatalf("@ inside a value hid the directive: %v", items)
	}
}

func TestBackslashDoesNotShiftLaterFields(t *testing.T) {
	// A bare backslash is literal, so it must not escape the space between fields.
	ctx := analyzeLine(t, `# @compare Dév\ One group=api base=On`)
	if ctx.Query != "On" || argKey(ctx) != "base" {
		t.Fatalf("caret context = %+v", ctx)
	}
	if !ctx.completed.holds(compareTarget, "One") || !ctx.completed.holds("group", "api") {
		t.Fatalf("backslash shifted subsequent values: %+v", ctx.completed)
	}
	items := directiveSource{}.Provide(ctx, Scope{})
	if len(items) != 1 || items[0].InsertText() != "One" {
		t.Fatalf("baseline values = %+v", items)
	}
}

func TestPathContextPreservesBackslashes(t *testing.T) {
	const path = `C:\modules\help`
	ctx := analyzeLine(t, "# @use "+path)
	if ctx.Kind != KindPath || ctx.Query != path {
		t.Fatalf("path context = %+v", ctx)
	}
}
