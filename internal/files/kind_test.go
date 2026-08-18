package files

import "testing"

// The zero Kind must not name a real kind: Entry{} and a rejected Classify
// both carry it, and either would otherwise read as a request file.
func TestZeroKindIsUnknown(t *testing.T) {
	var zero Kind
	if zero != KindUnknown {
		t.Fatalf("zero Kind = %v, want KindUnknown", zero)
	}
	if (Entry{}).Kind != KindUnknown {
		t.Fatalf("zero Entry kind = %v, want KindUnknown", (Entry{}).Kind)
	}
	if kind, ok := ClassifyWorkspace("notes.txt"); ok || kind != KindUnknown {
		t.Fatalf("ClassifyWorkspace(notes.txt) = %v, %v, want KindUnknown, false", kind, ok)
	}
	if kind, ok := ClassifyRequest("notes.txt"); ok || kind != KindUnknown {
		t.Fatalf("ClassifyRequest(notes.txt) = %v, %v, want KindUnknown, false", kind, ok)
	}
}

func TestClassifyRequestRejectsWorkspaceOnlyKinds(t *testing.T) {
	for _, path := range []string{"resterm.env.json", "query.graphql", "payload.json", "pre.js"} {
		if kind, ok := ClassifyRequest(path); ok {
			t.Fatalf("ClassifyRequest(%q) = %v, true, want false", path, kind)
		}
	}
}

func TestExtensionMatchIsCaseInsensitive(t *testing.T) {
	if kind, ok := ClassifyWorkspace("REQUESTS.HTTP"); !ok || kind != KindRequest {
		t.Fatalf("ClassifyWorkspace(REQUESTS.HTTP) = %v, %v, want KindRequest, true", kind, ok)
	}
	if !IsRequest("A.Rest") {
		t.Fatal("IsRequest(A.Rest) = false, want true")
	}
}

func TestKindString(t *testing.T) {
	tests := []struct {
		name string
		kind Kind
		want string
	}{
		{name: "request", kind: KindRequest, want: "request"},
		{name: "script", kind: KindScript, want: "script"},
		{name: "env", kind: KindEnv, want: "env"},
		{name: "graphql", kind: KindGraphQL, want: "graphql"},
		{name: "json", kind: KindJSON, want: "json"},
		{name: "javascript", kind: KindJavaScript, want: "javascript"},
		{name: "zero", kind: KindUnknown, want: "unknown"},
		{name: "out of range", kind: Kind(99), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKindBadgeLabel(t *testing.T) {
	tests := []struct {
		kind Kind
		want string
	}{
		{kind: KindUnknown, want: ""},
		{kind: KindRequest, want: ""},
		{kind: KindScript, want: ""},
		{kind: KindEnv, want: "ENV"},
		{kind: KindGraphQL, want: ""},
		{kind: KindJSON, want: ""},
		{kind: KindJavaScript, want: ""},
		{kind: Kind(99), want: ""},
	}

	for _, tt := range tests {
		if got := tt.kind.BadgeLabel(); got != tt.want {
			t.Fatalf("BadgeLabel(%v) = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestClassifyWorkspacePrecedence(t *testing.T) {
	tests := []struct {
		path string
		want Kind
		ok   bool
	}{
		{path: "requests.http", want: KindRequest, ok: true},
		{path: "requests.rest", want: KindRequest, ok: true},
		{path: "helpers.rts", want: KindScript, ok: true},
		{path: "resterm.env.json", want: KindEnv, ok: true},
		{path: "rest-client.env.json", want: KindEnv, ok: true},
		{path: "http-client.env.json", want: KindEnv, ok: true},
		{path: "http-client.private.env.json", want: KindEnv, ok: true},
		{path: "payload.json", want: KindJSON, ok: true},
		{path: "query.graphql", want: KindGraphQL, ok: true},
		{path: "query.gql", want: KindGraphQL, ok: true},
		{path: "pre.js", want: KindJavaScript, ok: true},
		{path: "pre.mjs", want: KindJavaScript, ok: true},
		{path: "pre.cjs", want: KindJavaScript, ok: true},
		{path: "notes.txt", ok: false},
	}

	for _, tt := range tests {
		got, ok := ClassifyWorkspace(tt.path)
		if ok != tt.ok {
			t.Fatalf("ClassifyWorkspace(%q) ok = %v, want %v", tt.path, ok, tt.ok)
		}
		if got != tt.want {
			t.Fatalf("ClassifyWorkspace(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}
