package vars

import (
	"testing"
)

func TestResolverTraceRecordsWinnerAndShadowedProviders(t *testing.T) {
	t.Parallel()

	r := NewResolver(
		NewMapProvider("env", map[string]string{"token": "env-token"}),
		NewMapProvider("file", map[string]string{"token": "file-token"}),
	)
	tr := NewTrace()
	r.SetTrace(tr)

	out, err := r.ExpandTemplates("{{token}}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "env-token" {
		t.Fatalf("expected env-token, got %q", out)
	}

	items := tr.Items()
	if len(items) != 1 {
		t.Fatalf("expected 1 trace item, got %d", len(items))
	}
	if items[0].Source != "env" {
		t.Fatalf("expected env source, got %q", items[0].Source)
	}
	if len(items[0].Shadowed) != 1 || items[0].Shadowed[0] != "file" {
		t.Fatalf("expected file to be shadowed, got %v", items[0].Shadowed)
	}
}

func TestResolverTraceRecordsDynamicAndMissingVariables(t *testing.T) {
	t.Parallel()

	r := NewResolver()
	tr := NewTrace()
	r.SetTrace(tr)

	out, err := r.ExpandTemplates("{{$timestamp}} {{missing}}")
	if err == nil {
		t.Fatalf("expected missing variable error")
	}
	if out == "{{$timestamp}} {{missing}}" {
		t.Fatalf("expected dynamic helper to expand, got %q", out)
	}

	items := tr.Items()
	if len(items) != 2 {
		t.Fatalf("expected 2 trace items, got %d", len(items))
	}

	var dyn, miss *ResolveTrace
	for i := range items {
		it := items[i]
		switch it.Name {
		case "$timestamp":
			dyn = &it
		case "missing":
			miss = &it
		}
	}
	if dyn == nil {
		t.Fatalf("expected dynamic trace item, got %v", items)
	}
	if !dyn.Dynamic || dyn.Source != "dynamic" {
		t.Fatalf("expected dynamic trace source, got %+v", *dyn)
	}
	if miss == nil {
		t.Fatalf("expected missing trace item, got %v", items)
	}
	if !miss.Missing {
		t.Fatalf("expected missing item to be marked missing, got %+v", *miss)
	}
}
