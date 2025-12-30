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

func TestCollectMetadataIncludesValidKeys(t *testing.T) {
	grpcReq := &restfile.GRPCRequest{
		Metadata: []restfile.MetadataPair{
			{Key: "X.Trace-Id", Value: "a"},
		},
	}
	req := &restfile.Request{
		Headers: http.Header{
			"X-Req-Id": []string{"b"},
		},
	}

	pairs, err := collectMetadata(grpcReq, req)
	if err != nil {
		t.Fatalf("collect metadata: %v", err)
	}
	got := pairMap(pairs)
	if firstVal(got["x.trace-id"]) != "a" {
		t.Fatalf("expected x.trace-id metadata, got %#v", got["x.trace-id"])
	}
	if firstVal(got["x-req-id"]) != "b" {
		t.Fatalf("expected x-req-id header metadata, got %#v", got["x-req-id"])
	}
}

func TestCollectMetadataReservedTimeout(t *testing.T) {
	grpcReq := &restfile.GRPCRequest{
		Metadata: []restfile.MetadataPair{
			{Key: "grpc-timeout", Value: "1s"},
		},
	}
	_, err := collectMetadata(grpcReq, &restfile.Request{})
	if err == nil {
		t.Fatalf("expected reserved timeout error")
	}
	if !strings.Contains(err.Error(), "@timeout") {
		t.Fatalf("expected @timeout guidance, got %v", err)
	}
}

func TestCollectMetadataReservedHeader(t *testing.T) {
	req := &restfile.Request{
		Headers: http.Header{
			"Content-Type": []string{"application/json"},
		},
	}
	_, err := collectMetadata(&restfile.GRPCRequest{}, req)
	if err == nil {
		t.Fatalf("expected reserved header error")
	}
	if !strings.Contains(err.Error(), "from headers") {
		t.Fatalf("expected header context, got %v", err)
	}
}

func TestCollectMetadataInvalidKey(t *testing.T) {
	grpcReq := &restfile.GRPCRequest{
		Metadata: []restfile.MetadataPair{
			{Key: "bad key", Value: "skip"},
		},
	}
	_, err := collectMetadata(grpcReq, &restfile.Request{})
	if err == nil {
		t.Fatalf("expected invalid key error")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("expected invalid character detail, got %v", err)
	}
}

func TestResolveMessagePrefersExpanded(t *testing.T) {
	client := NewClient()
	grpcReq := &restfile.GRPCRequest{
		MessageFile:        "msg.json",
		MessageExpanded:    `{"id":"abc"}`,
		MessageExpandedSet: true,
	}
	got, err := client.resolveMessage(grpcReq, "")
	if err != nil {
		t.Fatalf("resolve message: %v", err)
	}
	if got != `{"id":"abc"}` {
		t.Fatalf("expected expanded message, got %q", got)
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
