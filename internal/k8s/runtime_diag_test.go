package k8s

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	kruntime "k8s.io/apimachinery/pkg/util/runtime"
)

func TestParsePFErr(t *testing.T) {
	raw := `an error occurred forwarding 50611 -> 8080: error forwarding port 8080 to pod 629785d9c3915a4f21595feaa2216106a15fa37716b43a2de90aeaaa363f9f0f, uid : failed to execute portforward in network namespace "/var/run/netns/cni-9a50e3a4-5ae8-9c80-cb35-f761e28eab2a": failed to connect to localhost:8080 inside namespace "629785d9c3915a4f21595feaa2216106a15fa37716b43a2de90aeaaa363f9f0f"`

	pf, ok := parsePFErr(raw)
	if !ok {
		t.Fatalf("expected port-forward parse to succeed")
	}
	if pf.local != 50611 || pf.remote != 8080 {
		t.Fatalf("unexpected local/remote ports: %+v", pf)
	}
	if !strings.Contains(pf.summary, "k8s port-forward 50611->8080:") {
		t.Fatalf("expected summary prefix, got %q", pf.summary)
	}
	if !strings.Contains(pf.summary, "failed to connect to localhost:8080") {
		t.Fatalf("expected connection failure detail, got %q", pf.summary)
	}
	if strings.Contains(pf.summary, `inside namespace "`) {
		t.Fatalf("expected namespace id redaction, got %q", pf.summary)
	}
}

func TestAnnotateRequestErrorWithRecentPFErr(t *testing.T) {
	resetRuntimeDiagForTest()

	now := time.Now()
	start := now.Add(-250 * time.Millisecond)
	raw := `an error occurred forwarding 50611 -> 8080: error forwarding port 8080 to pod x, uid : failed to connect to localhost:8080 inside namespace "x"`
	_, reqDiag := bindTestRequestDiag(50611, 8080)
	pushRuntimeErr(now, errors.New(raw))

	err := AnnotateRequestError(io.EOF, start, reqDiag)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected wrapped EOF, got %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "k8s port-forward 50611->8080:") {
		t.Fatalf("expected k8s annotation, got %q", msg)
	}
	if !strings.Contains(msg, "failed to connect to localhost:8080") {
		t.Fatalf("expected connect detail, got %q", msg)
	}
}

func TestAnnotateRequestErrorSkipsStalePFErr(t *testing.T) {
	resetRuntimeDiagForTest()

	raw := `an error occurred forwarding 50611 -> 8080: error forwarding port 8080 to pod x, uid : failed to connect to localhost:8080`
	_, reqDiag := bindTestRequestDiag(50611, 8080)
	pushRuntimeErr(time.Now().Add(-2*diagMaxAge), errors.New(raw))

	err := AnnotateRequestError(io.EOF, time.Now().Add(-time.Second), reqDiag)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF unchanged, got %v", err)
	}
	if strings.Contains(err.Error(), "k8s port-forward") {
		t.Fatalf("expected no annotation for stale errors, got %q", err.Error())
	}
}

func TestAnnotateRequestErrorFromRuntimeHandleError(t *testing.T) {
	resetRuntimeDiagForTest()
	ensureRuntimeDiagInstalled()
	_, reqDiag := bindTestRequestDiag(41111, 8443)

	start := time.Now().Add(-time.Second)
	kruntime.HandleError(
		errors.New(
			`an error occurred forwarding 41111 -> 8443: error forwarding port 8443 to pod x, uid : failed to connect to localhost:8443 inside namespace "x"`,
		),
	)

	err := AnnotateRequestError(io.EOF, start, reqDiag)
	if !strings.Contains(err.Error(), "k8s port-forward 41111->8443:") {
		t.Fatalf("expected runtime.HandleError capture, got %q", err.Error())
	}
}

func TestAnnotateRequestErrorDoesNotCrossCollectors(t *testing.T) {
	resetRuntimeDiagForTest()

	now := time.Now()
	start := now.Add(-250 * time.Millisecond)
	raw := `an error occurred forwarding 50612 -> 8080: error forwarding port 8080 to pod x, uid : failed to connect to localhost:8080 inside namespace "x"`

	_, first := bindTestRequestDiag(50611, 8080)
	_, second := bindTestRequestDiag(50612, 8080)
	pushRuntimeErr(now, errors.New(raw))

	errOne := AnnotateRequestError(io.EOF, start, first)
	if strings.Contains(errOne.Error(), "k8s port-forward") {
		t.Fatalf("expected first request diag to stay clean, got %q", errOne.Error())
	}

	errTwo := AnnotateRequestError(io.EOF, start, second)
	if !strings.Contains(errTwo.Error(), "k8s port-forward 50612->8080:") {
		t.Fatalf("expected second request diag annotation, got %q", errTwo.Error())
	}
}

func TestBuildRuntimeDiagErrorHandlersPreservesNonRuntimeHandlers(t *testing.T) {
	got := buildRuntimeDiagErrorHandlers(
		[]kruntime.ErrorHandler{
			testErrHandlerA,
			testErrHandlerB,
			testErrHandlerC,
		},
	)
	want := []kruntime.ErrorHandler{
		captureRuntimeErr,
		testErrHandlerA,
		testErrHandlerB,
		testErrHandlerC,
	}
	assertHandlerPtrs(t, got, want)
}

func TestBuildRuntimeDiagErrorHandlersDropsRuntimeLogger(t *testing.T) {
	got := buildRuntimeDiagErrorHandlers(kruntime.ErrorHandlers)
	if len(got) == 0 {
		t.Fatal("expected runtime handlers")
	}
	if handlerPtr(got[0]) != handlerPtr(captureRuntimeErr) {
		t.Fatalf("expected capture handler first")
	}
	for i := 1; i < len(got); i++ {
		if isDefaultRuntimeLogHandler(got[i]) {
			t.Fatalf("unexpected default runtime log handler at %d", i)
		}
	}
}

func assertHandlerPtrs(t *testing.T, got, want []kruntime.ErrorHandler) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected handler count: got=%d want=%d", len(got), len(want))
	}
	for i := range want {
		gptr := handlerPtr(got[i])
		wptr := handlerPtr(want[i])
		if gptr != wptr {
			t.Fatalf("handler[%d] mismatch: got=%#x want=%#x", i, gptr, wptr)
		}
	}
}

func handlerPtr(h kruntime.ErrorHandler) uintptr {
	if h == nil {
		return 0
	}
	return reflect.ValueOf(h).Pointer()
}

func testErrHandlerA(_ context.Context, _ error, _ string, _ ...any) {}
func testErrHandlerB(_ context.Context, _ error, _ string, _ ...any) {}
func testErrHandlerC(_ context.Context, _ error, _ string, _ ...any) {}

func bindTestRequestDiag(local, remote int) (context.Context, *RequestDiag) {
	ctx, reqDiag := BindRequestContext(context.Background())
	bindRequestDiag(ctx, newDiagCollector(local, remote, diagCap))
	return ctx, reqDiag
}

func resetRuntimeDiagForTest() {
	rtDiag = newDiagState()
}
