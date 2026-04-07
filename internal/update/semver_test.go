package update

import "testing"

func TestParseSemver(t *testing.T) {
	v, err := parseSemver("v1.2.3")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if v.maj != 1 || v.min != 2 || v.patch != 3 {
		t.Fatalf("unexpected values: %+v", v)
	}
}

func TestParseSemverInvalid(t *testing.T) {
	if _, err := parseSemver("x.y.z"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSemverCompare(t *testing.T) {
	less, err := parseSemver("1.2.3")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	more, err := parseSemver("1.2.4")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if !less.lt(more) {
		t.Fatal("expected 1.2.3 to sort below 1.2.4")
	}
	if _, err := parseSemver("1.a"); err == nil {
		t.Fatal("expected error")
	}
}
