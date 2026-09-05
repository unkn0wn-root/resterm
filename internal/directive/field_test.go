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
		if got := field.Eq >= 0; got != want[i].option {
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
