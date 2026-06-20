package intellisense

import (
	"strings"
	"testing"
)

func TestMethodSource(t *testing.T) {
	all := methodSource{}.Provide(Context{Kind: KindMethod}, Scope{})
	for _, label := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "WS", "WSS", "GRPC"} {
		if !contains(all, label) {
			t.Fatalf("method catalog missing %q", label)
		}
	}
	for _, it := range (methodSource{}).Provide(Context{Kind: KindMethod, Query: "p"}, Scope{}) {
		if !strings.HasPrefix(strings.ToLower(it.Label), "p") {
			t.Fatalf("expected p* method, got %q", it.Label)
		}
	}
	if (methodSource{}).Provide(Context{Kind: KindHeaderName}, Scope{}) != nil {
		t.Fatal("method source should ignore non-method kinds")
	}
}

func TestHeaderSource(t *testing.T) {
	names := headerSource{}.Provide(Context{Kind: KindHeaderName, Query: "content"}, Scope{})
	if !contains(names, "Content-Type") {
		t.Fatalf("header names missing Content-Type: %v", names)
	}
	for _, it := range names {
		if it.Label == "Content-Type" && it.Insert != "Content-Type:" {
			t.Fatalf("expected Content-Type insert with colon, got %q", it.Insert)
		}
	}

	values := headerSource{}.Provide(
		Context{Kind: KindHeaderValue, Directive: "content-type", Query: "app"},
		Scope{},
	)
	if !contains(values, "application/json") {
		t.Fatalf("content-type values missing application/json: %v", values)
	}
	if got := (headerSource{}).Provide(
		Context{Kind: KindHeaderValue, Directive: "x-unknown"},
		Scope{},
	); got != nil {
		t.Fatalf("expected nil for unknown header values, got %v", got)
	}
}

func TestVariableSourceMergesScopeAndBuiltins(t *testing.T) {
	sc := Scope{Variables: []VarRef{
		{Name: "host", Origin: "file"},
		{Name: "token", Origin: "global", Secret: true},
	}}

	all := variableSource{}.Provide(Context{Kind: KindVariable}, sc)
	for _, label := range []string{"host", "token", "$uuid", "$timestamp", "$randomInt"} {
		if !contains(all, label) {
			t.Fatalf("variable suggestions missing %q: %v", label, all)
		}
	}
	for _, it := range all {
		if it.Label == "token" && it.Summary != "global (secret)" {
			t.Fatalf("expected secret marker on token, got %q", it.Summary)
		}
	}

	dollars := variableSource{}.Provide(Context{Kind: KindVariable, Query: "$ti"}, sc)
	if !contains(dollars, "$timestamp") || contains(dollars, "host") {
		t.Fatalf("expected only $ti* builtins, got %v", dollars)
	}
}

func TestEngineDispatchesByKind(t *testing.T) {
	e := New()
	sc := Scope{Variables: []VarRef{{Name: "host", Origin: "file"}}}

	if items := e.Suggest(Context{Kind: KindMethod, Query: "g"}, sc); !contains(items, "GET") {
		t.Fatalf("engine did not route method context: %v", items)
	}
	if items := e.Suggest(Context{Kind: KindVariable, Query: "ho"}, sc); !contains(items, "host") {
		t.Fatalf("engine did not route variable context: %v", items)
	}
	if items := e.Suggest(
		Context{Kind: KindDirective, Query: "aut"},
		sc,
	); !contains(
		items,
		"@auth",
	) {
		t.Fatalf("engine did not route directive context: %v", items)
	}
	if items := e.Suggest(Context{Kind: KindNone}, sc); items != nil {
		t.Fatalf("engine should return nil for KindNone, got %v", items)
	}
}
