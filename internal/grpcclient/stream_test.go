package grpcclient

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

func streamingRequest(t *testing.T) (*restfile.GRPCRequest, Options) {
	t.Helper()
	gr, opt := descriptorRequest(t)
	gr.FullMethod = "/pkg.Svc/ServerStream"
	return gr, opt
}

func TestStreamResponseDropsWireContentType(t *testing.T) {
	gr, opt := streamingRequest(t)
	conn := &stubConn{stream: &stubStream{recvCount: 1}}

	resp, err := stubClient(conn).Execute(context.Background(), &restfile.Request{GRPC: gr}, opt, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.WireContentType != "" {
		t.Fatalf("WireContentType = %q, want empty for a stream", resp.WireContentType)
	}
	if len(resp.Wire) != 0 {
		t.Fatalf("Wire = %q, want empty for a stream", resp.Wire)
	}

	body, ct := resp.RawBody()
	if ct != defaultContentType {
		t.Fatalf("RawBody type = %q, want %q", ct, defaultContentType)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(body)), "[") {
		t.Fatalf("RawBody = %q, want the json transcript", body)
	}
}

func TestStreamSessionCancelStopsRPC(t *testing.T) {
	gr, opt := streamingRequest(t)
	conn := &stubConn{stream: &stubStream{blockCtx: true}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = stubClient(conn).Execute(
			context.Background(),
			&restfile.Request{GRPC: gr},
			opt,
			func(s *stream.Session) { s.Cancel() },
		)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("session.Cancel did not stop the rpc")
	}
}

func TestStreamHookPublishesEvents(t *testing.T) {
	gr, opt := streamingRequest(t)
	conn := &stubConn{stream: &stubStream{recvCount: 2}}

	var session *stream.Session
	_, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		func(s *stream.Session) { session = s },
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if session == nil {
		t.Fatal("the hook never received a session")
	}

	evs := session.EventsSnapshot()
	var recv, summary int
	for _, ev := range evs {
		if ev.Metadata[MetaMethod] != gr.FullMethod {
			t.Fatalf("event method = %q, want %q", ev.Metadata[MetaMethod], gr.FullMethod)
		}
		switch ev.Direction {
		case stream.DirReceive:
			if ev.Metadata[MetaMsgType] != "pkg.Msg" {
				t.Fatalf("message type = %q, want pkg.Msg", ev.Metadata[MetaMsgType])
			}
			if got := ev.Metadata[MetaMsgIndex]; got != strconv.Itoa(recv) {
				t.Fatalf("message index = %q, want %d", got, recv)
			}
			recv++
		case stream.DirNA:
			summary++
			if ev.Metadata[MetaStatus] != "OK" {
				t.Fatalf("summary status = %q, want OK", ev.Metadata[MetaStatus])
			}
		}
	}
	if recv != 2 {
		t.Fatalf("received %d events, want 2", recv)
	}
	if summary != 1 {
		t.Fatalf("published %d summaries, want 1", summary)
	}
}

func TestExecuteUnaryRejectsJSONArrayBody(t *testing.T) {
	gr, opt := descriptorRequest(t)
	gr.Message = `[{}, {}]`

	_, err := stubClient(&stubConn{}).
		Execute(context.Background(), &restfile.Request{GRPC: gr}, opt, nil)
	if err == nil {
		t.Fatal("expected an array body on a unary method to be rejected")
	}
	if !strings.Contains(err.Error(), "expects a single message") {
		t.Fatalf("err = %v, want the single message guidance", err)
	}
}
