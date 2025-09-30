package grpcclient

import (
	"testing"

	"github.com/unkn0wn-root/resterm/pkg/restfile"
)

func TestShouldUsePlaintextHonoursRequestOverride(t *testing.T) {
	opts := Options{DefaultPlaintext: false, DefaultPlaintextSet: true}
	req := &restfile.GRPCRequest{Plaintext: true, PlaintextSet: true}

	if !shouldUsePlaintext(req, opts) {
		t.Fatalf("expected request override to force plaintext")
	}
}

func TestShouldUsePlaintextFallsBackToOptions(t *testing.T) {
	opts := Options{DefaultPlaintext: true, DefaultPlaintextSet: true}
	req := &restfile.GRPCRequest{}

	if !shouldUsePlaintext(req, opts) {
		t.Fatalf("expected fallback to options when request unset")
	}
}

func TestShouldUsePlaintextHandlesExplicitFalse(t *testing.T) {
	opts := Options{DefaultPlaintext: true, DefaultPlaintextSet: true}
	req := &restfile.GRPCRequest{Plaintext: false, PlaintextSet: true}

	if shouldUsePlaintext(req, opts) {
		t.Fatalf("expected explicit false to disable plaintext")
	}
}
