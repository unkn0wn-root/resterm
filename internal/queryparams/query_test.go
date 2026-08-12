package queryparams

import (
	"reflect"
	"testing"
)

func TestParseTreatsWhitespaceAsData(t *testing.T) {
	got, err := Parse("? key = value &key=two")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := Values{" key ": {" value "}, "key": {"two"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse() = %#v, want %#v", got, want)
	}
}

func TestParseAndFromURLHaveSeparateContracts(t *testing.T) {
	raw, err := Parse("https://example.test/p?a=1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := raw["https://example.test/p?a"]; !ok {
		t.Fatalf("Parse interpreted raw query text as a URL: %#v", raw)
	}

	got, err := FromURL("https://example.test/p?a=1&a=2")
	if err != nil {
		t.Fatalf("FromURL: %v", err)
	}
	want := Values{"a": {"1", "2"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FromURL() = %#v, want %#v", got, want)
	}
}

// A malformed escape is reported rather than dropped. url.URL.Query would
// return the query without the pair it could not decode, which reads as a URL
// that never carried it.
func TestFromURLReportsMalformedEscapes(t *testing.T) {
	got, err := FromURL("https://example.test/p?bad=%zz&keep=1")
	if err == nil {
		t.Fatalf("FromURL() = %#v, want an error", got)
	}
	if got != nil {
		t.Fatalf("FromURL() = %#v after an error, want nil", got)
	}
}
