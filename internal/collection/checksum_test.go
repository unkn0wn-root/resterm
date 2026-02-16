package collection

import (
	"strings"
	"testing"
)

func TestSHA256Digest(t *testing.T) {
	data := []byte("hello")
	got := SumSHA256(data)

	if got.Alg != AlgSHA256 {
		t.Fatalf("alg=%q want %q", got.Alg, AlgSHA256)
	}
	if !ValidSHA256(got.Value) {
		t.Fatalf("sum should be valid sha256: %q", got.Value)
	}
	if !VerifyDigest(data, got) {
		t.Fatalf("expected digest verification success")
	}
	if VerifyDigest([]byte("HELLO"), got) {
		t.Fatalf("expected digest verification failure for modified payload")
	}
}

func TestVerifyDigestNormalizesInput(t *testing.T) {
	data := []byte("hello")
	d := SumSHA256(data)
	d.Alg = " SHA256 "
	d.Value = " " + d.Value + " "
	if !VerifyDigest(data, d) {
		t.Fatalf("expected normalized digest verification success")
	}
}

func TestValidSHA256AcceptsUppercaseHex(t *testing.T) {
	data := []byte("hello")
	d := SumSHA256(data)
	upper := strings.ToUpper(d.Value)

	if !ValidSHA256(upper) {
		t.Fatalf("expected uppercase digest to be accepted: %q", upper)
	}
}
