package bodyfmt

import (
	"net/http"
	"reflect"
	"testing"
)

func TestHeaderFieldsSortsNamesAndValues(t *testing.T) {
	headers := http.Header{
		"X-B": {"2", "1"},
		"X-A": {"z"},
	}
	want := []HeaderField{
		{Name: "X-A", Value: "z"},
		{Name: "X-B", Value: "1, 2"},
	}
	if got := HeaderFields(headers); !reflect.DeepEqual(got, want) {
		t.Fatalf("HeaderFields()=%v, want %v", got, want)
	}
	if got := headers["X-B"]; !reflect.DeepEqual(got, []string{"2", "1"}) {
		t.Fatalf("HeaderFields mutated caller values: %v", got)
	}
}

func TestFormatHeadersSortsNamesAndValues(t *testing.T) {
	headers := http.Header{
		"X-B": {"2", "1"},
		"X-A": {"z"},
	}
	got := FormatHeaders(headers)
	want := "X-A: z\nX-B: 1, 2"
	if got != want {
		t.Fatalf("FormatHeaders()=%q, want %q", got, want)
	}
}

func TestFormatHeadersKeepsValuelessNames(t *testing.T) {
	got := FormatHeaders(http.Header{"X-Empty": {""}})
	if got != "X-Empty:" {
		t.Fatalf("FormatHeaders()=%q, want %q", got, "X-Empty:")
	}
}

func TestFormatHeadersEmpty(t *testing.T) {
	if got := FormatHeaders(nil); got != "" {
		t.Fatalf("FormatHeaders(nil)=%q, want empty", got)
	}
}
