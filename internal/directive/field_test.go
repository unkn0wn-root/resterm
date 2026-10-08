package directive

import (
	"slices"
	"testing"
)

func TestScanFieldsKeepsValuesWithTheirSource(t *testing.T) {
	const input = `"Dév One" using=Create\ User json={"name": "ø"} latency=normal(1s, 2s) key="C:\tmp\file"`
	want := []struct {
		raw, value string
		option     bool
	}{
		{`"Dév One"`, "Dév One", false},
		{`using=Create\`, `using=Create\`, true},
		{`User`, "User", false},
		{`json={"name": "ø"}`, `json={"name": "ø"}`, true},
		{`latency=normal(1s, 2s)`, `latency=normal(1s, 2s)`, true},
		{`key="C:\tmp\file"`, `key=C:\tmp\file`, true},
	}
	fields := slices.Collect(ScanFields(input))
	if len(fields) != len(want) {
		t.Fatalf("fields = %+v, want %d", fields, len(want))
	}
	for i, field := range fields {
		if raw := input[field.Start:field.End]; raw != want[i].raw || field.Value != want[i].value {
			t.Errorf("field %d: source %q, value %q; want %+v", i, raw, field.Value, want[i])
		}
		if got := field.Op == OpEq; got != want[i].option {
			t.Errorf("field %d: option = %v", i, got)
		}
	}
}

func TestScanFieldsCanStopAndRestart(t *testing.T) {
	fields := ScanFields(`first=1 second="unfinished`)
	for field := range fields {
		if field.Value != "first=1" {
			t.Fatalf("first field = %+v", field)
		}
		break
	}
	all := slices.Collect(fields)
	if len(all) != 2 || all[1].Value != "second=unfinished" {
		t.Fatalf("restarted fields = %+v", all)
	}
}

func TestIsOption(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"host=jump":            true,
		"local-port=1":         true,
		"bare":                 false,
		"=orphan":              false,
		"has space=1":          false,
		"expect.status":        false,
		"last.statusCode==200": false,
		"a!=b":                 false,
		"a>=b":                 false,
	}
	for input, want := range tests {
		if got := isOption(input); got != want {
			t.Fatalf("isOption(%q) = %t, want %t", input, got, want)
		}
	}
}

func TestFieldSpans(t *testing.T) {
	t.Parallel()

	input := `"file edge" host=jump json={"a":"b c"} last==200 timeout="5 s" path=a\ b persist`
	want := []struct {
		field string
		key   string
	}{
		{field: `"file edge"`},
		{field: "host=jump", key: "host"},
		{field: `json={"a":"b c"}`, key: "json"},
		{field: "last==200"},
		{field: `timeout="5 s"`, key: "timeout"},
		{field: `path=a\ b`, key: "path"},
		{field: "persist"},
	}

	spans := FieldSpans(input)
	if len(spans) != len(want) {
		t.Fatalf("FieldSpans(%q) returned %d spans, want %d", input, len(spans), len(want))
	}
	for i, w := range want {
		f := spans[i]
		if got := input[f.Start:f.End]; got != w.field {
			t.Fatalf("span %d = %q, want %q", i, got, w.field)
		}
		if w.key == "" {
			if f.Op != OpNone {
				t.Fatalf("span %d (%q) has Op %q, want positional", i, w.field, f.Op)
			}
			continue
		}
		if f.Op != OpEq || input[f.Start:f.At] != w.key {
			t.Fatalf("span %d (%q) = %+v, want key %q", i, w.field, f, w.key)
		}
	}
}

func TestScanFieldsReadsEveryOperator(t *testing.T) {
	t.Parallel()

	for _, op := range ops {
		f := slices.Collect(ScanFields("k" + op.String() + "v"))[0]
		if f.Op != op || f.At != 1 || f.ValueStart() != 1+len(op.String()) {
			t.Fatalf("k%sv = %+v", op, f)
		}
	}
}
