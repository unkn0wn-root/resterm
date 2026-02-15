package k8s

import (
	"errors"
	"io"
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
	pushRuntimeErr(now, errors.New(raw))

	err := AnnotateRequestError(io.EOF, start)
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
	pushRuntimeErr(time.Now().Add(-2*diagMaxAge), errors.New(raw))

	err := AnnotateRequestError(io.EOF, time.Now().Add(-time.Second))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF unchanged, got %v", err)
	}
	if strings.Contains(err.Error(), "k8s port-forward") {
		t.Fatalf("expected no annotation for stale errors, got %q", err.Error())
	}
}

func TestAnnotateRequestErrorFromRuntimeHandleError(t *testing.T) {
	resetRuntimeDiagForTest()

	start := time.Now().Add(-time.Second)
	kruntime.HandleError(
		errors.New(
			`an error occurred forwarding 41111 -> 8443: error forwarding port 8443 to pod x, uid : failed to connect to localhost:8443 inside namespace "x"`,
		),
	)

	err := AnnotateRequestError(io.EOF, start)
	if !strings.Contains(err.Error(), "k8s port-forward 41111->8443:") {
		t.Fatalf("expected runtime.HandleError capture, got %q", err.Error())
	}
}

func resetRuntimeDiagForTest() {
	rtDiag.mu.Lock()
	rtDiag.buf = newRing[diagRecord](diagCap)
	rtDiag.mu.Unlock()
}
