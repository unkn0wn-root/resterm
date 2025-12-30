package grpcclient

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

func TestShouldUsePlaintextDisabledWhenTLSConfigured(t *testing.T) {
	opts := Options{RootCAs: []string{"ca.pem"}}
	req := &restfile.GRPCRequest{}

	if shouldUsePlaintext(req, opts) {
		t.Fatalf("expected TLS settings to disable plaintext")
	}
}

func TestFetchDescriptorsReflectionError(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	_, err = fetchDescriptorsViaReflection(
		context.Background(),
		conn,
		"/grpc.testing.MissingService/MissingMethod",
	)
	if err == nil {
		t.Fatalf("expected reflection error")
	}
	if !strings.Contains(err.Error(), "grpc reflection error") {
		t.Fatalf("expected reflection error detail, got %v", err)
	}
}

func TestCollectMetadataFiltersHeaders(t *testing.T) {
	grpcReq := &restfile.GRPCRequest{
		Metadata: []restfile.MetadataPair{
			{Key: "x-trace-id", Value: "a"},
			{Key: "grpc-timeout", Value: "1s"},
			{Key: "bad key", Value: "skip"},
		},
	}
	req := &restfile.Request{
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
			"User-Agent":   []string{"agent"},
			"X-Req-Id":     []string{"b"},
			"Grpc-Timeout": []string{"2s"},
			"Bad Key":      []string{"drop"},
		},
	}

	got := pairMap(collectMetadata(grpcReq, req))
	if firstVal(got["x-trace-id"]) != "a" {
		t.Fatalf("expected x-trace-id metadata, got %#v", got["x-trace-id"])
	}
	if firstVal(got["x-req-id"]) != "b" {
		t.Fatalf("expected x-req-id header metadata, got %#v", got["x-req-id"])
	}
	if _, ok := got["grpc-timeout"]; ok {
		t.Fatalf("expected grpc-timeout metadata to be filtered")
	}
	if _, ok := got["content-type"]; ok {
		t.Fatalf("expected content-type header to be filtered")
	}
	if _, ok := got["user-agent"]; ok {
		t.Fatalf("expected user-agent header to be filtered")
	}
	if _, ok := got["bad key"]; ok {
		t.Fatalf("expected invalid keys to be filtered")
	}
}

func pairMap(pairs []string) map[string][]string {
	out := map[string][]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		key := pairs[i]
		out[key] = append(out[key], pairs[i+1])
	}
	return out
}

func firstVal(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
