package mock

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// A client may hold a connection that never sends a request, such as a browser
// preconnect or a spare dial from Go's Transport.
func TestCloseDoesNotWaitForAConnectionWithoutARequest(t *testing.T) {
	server, err := Start(
		"127.0.0.1:0",
		compileSource(t, "# @mock method=GET path=/value\nHTTP/1.1 200 OK\n\nok"),
		Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	idle, err := net.Dial("tcp", server.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idle.Close() }()
	// Accept is FIFO, so once a later connection is served, idle was accepted.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	if body := readBody(t, get(t, client, "http://"+server.Addr()+"/value", "")); body != "ok" {
		t.Fatalf("body = %q", body)
	}

	// Under the five seconds net/http lets a new connection sit before
	// http.Server.Shutdown treats it as idle.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil || ctx.Err() != nil {
		t.Fatalf("Close = %v, ctx = %v, want both nil", err, ctx.Err())
	}
}

func TestCloseLetsARequestInFlightFinish(t *testing.T) {
	server, err := Start(
		"127.0.0.1:0",
		compileSource(t, "# @mock method=GET path=/slow latency=300ms\nHTTP/1.1 200 OK\n\nslow"),
		Options{},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		res, err := http.Get("http://" + server.Addr() + "/slow")
		if err != nil {
			got <- result{err: err}
			return
		}
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		got <- result{string(body), err}
	}()
	for start := time.Now(); ; time.Sleep(time.Millisecond) {
		if n, _ := server.Count(context.Background(), RequestPattern{}); n > 0 {
			break
		}
		if time.Since(start) > 2*time.Second {
			t.Fatal("request never reached the server")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatalf("Close = %v", err)
	}
	if r := <-got; r.err != nil || r.body != "slow" {
		t.Fatalf("in-flight request = %q, %v, want it served", r.body, r.err)
	}
}
