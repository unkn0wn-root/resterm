package ssh

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestDialSSHHandshakeErrorHasNoCloseNoise(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"))
		_ = conn.Close()
	}()

	cfg := execConfig{
		Config: Config{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, User: "u", Timeout: 2 * time.Second},
		auth:   authSpec{pass: "p"},
	}

	_, err = dialSSH(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected a handshake error")
	}
	if errors.Is(err, net.ErrClosed) {
		t.Fatalf("handshake error carries a second close: %v", err)
	}
}
