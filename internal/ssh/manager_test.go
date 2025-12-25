package ssh

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

type fakeClient struct {
	dials    atomic.Int32
	requests atomic.Int32
	failDial bool
}

func (f *fakeClient) Dial(network, addr string) (net.Conn, error) {
	f.dials.Add(1)
	if f.failDial {
		return nil, errBoom
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

func (f *fakeClient) SendRequest(
	name string,
	wantReply bool,
	payload []byte,
) (bool, []byte, error) {
	f.requests.Add(1)
	return true, nil, nil
}

func (f *fakeClient) Close() error {
	return nil
}

type scriptedClient struct {
	results []error
	dials   atomic.Int32
	closed  atomic.Int32
}

func (s *scriptedClient) Dial(network, addr string) (net.Conn, error) {
	idx := int(s.dials.Add(1)) - 1
	if idx < len(s.results) {
		if err := s.results[idx]; err != nil {
			return nil, err
		}
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

func (s *scriptedClient) SendRequest(
	name string,
	wantReply bool,
	payload []byte,
) (bool, []byte, error) {
	return true, nil, nil
}

func (s *scriptedClient) Close() error {
	s.closed.Add(1)
	return nil
}

func TestDialNonPersistent(t *testing.T) {
	cfg := Cfg{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false}
	dials := atomic.Int32{}

	m := &Manager{
		cache: make(map[string]*entry),
		ttl:   time.Minute,
		now:   time.Now,
		dial: func(ctx context.Context, cfg Cfg) (Client, error) {
			dials.Add(1)
			return &fakeClient{}, nil
		},
	}

	conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if got := dials.Load(); got != 1 {
		t.Fatalf("expected 1 dial, got %d", got)
	}
}

func TestDialPersistentCaches(t *testing.T) {
	cfg := Cfg{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true, KeepAlive: 0}
	dials := atomic.Int32{}

	fc := &fakeClient{}
	m := &Manager{
		cache: make(map[string]*entry),
		ttl:   time.Minute,
		now:   time.Now,
		dial: func(ctx context.Context, cfg Cfg) (Client, error) {
			dials.Add(1)
			return fc, nil
		},
	}

	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "x:81")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn1.Close()
	_ = conn2.Close()

	if got := dials.Load(); got != 1 {
		t.Fatalf("expected 1 dial, got %d", got)
	}
	if got := fc.dials.Load(); got != 2 {
		t.Fatalf("expected 2 forwarded dials, got %d", got)
	}
}

func TestDialPersistentReconnectsAfterCachedFailure(t *testing.T) {
	cfg := Cfg{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true, KeepAlive: 0}
	first := &scriptedClient{results: []error{nil, errBoom}}
	second := &scriptedClient{}
	dials := atomic.Int32{}

	m := &Manager{
		cache: make(map[string]*entry),
		ttl:   time.Minute,
		now:   time.Now,
		dial: func(ctx context.Context, cfg Cfg) (Client, error) {
			switch dials.Add(1) {
			case 1:
				return first, nil
			case 2:
				return second, nil
			default:
				return nil, errors.New("unexpected dial")
			}
		},
	}

	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	_ = conn1.Close()

	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "x:81")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn2.Close()

	if got := dials.Load(); got != 2 {
		t.Fatalf("expected 2 ssh client dials, got %d", got)
	}
	if got := first.dials.Load(); got != 2 {
		t.Fatalf("expected cached client to handle 2 dial attempts, got %d", got)
	}
	if got := first.closed.Load(); got == 0 {
		t.Fatalf("expected cached client to be closed after failed dial")
	}
	if got := second.dials.Load(); got != 1 {
		t.Fatalf("expected new client to handle replacement dial, got %d", got)
	}

	key := cacheKey(cfg)
	m.mu.Lock()
	ent := m.cache[key]
	m.mu.Unlock()
	if ent == nil || ent.cli != second {
		t.Fatalf("expected cache to hold replacement client")
	}
}

func TestDialRetry(t *testing.T) {
	cfg := Cfg{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false, Retries: 2}
	count := atomic.Int32{}

	m := &Manager{
		cache: make(map[string]*entry),
		ttl:   time.Minute,
		now:   time.Now,
		dial: func(ctx context.Context, cfg Cfg) (Client, error) {
			if count.Add(1); count.Load() < 2 {
				return nil, errBoom
			}
			return &fakeClient{}, nil
		},
	}

	conn, err := m.DialContext(context.Background(), cfg, "tcp", "z:80")
	if err != nil {
		t.Fatalf("retry dial failed: %v", err)
	}
	_ = conn.Close()
	if got := count.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}

func TestDefaultKeyFallbackSkipsMissing(t *testing.T) {
	s := loadDefaultKey("")
	if s != nil {
		t.Fatalf("expected nil signer when no default key exists in sandbox")
	}
}

func TestKeepAliveStops(t *testing.T) {
	cfg := Cfg{
		Host:      "h",
		Port:      22,
		User:      "u",
		Pass:      "p",
		Persist:   true,
		KeepAlive: 5 * time.Millisecond,
	}
	fc := &fakeClient{}

	m := &Manager{
		cache: make(map[string]*entry),
		ttl:   time.Minute,
		now:   time.Now,
		dial: func(ctx context.Context, cfg Cfg) (Client, error) {
			return fc, nil
		},
	}

	conn, err := m.DialContext(context.Background(), cfg, "tcp", "k:80")
	if err != nil {
		t.Fatalf("keepalive dial err: %v", err)
	}
	_ = conn.Close()

	time.Sleep(20 * time.Millisecond)
	_ = m.Close()

	if fc.requests.Load() == 0 {
		t.Fatalf("expected keepalive requests to fire")
	}
}

func TestDialRetryHonorsCancelledContext(t *testing.T) {
	cfg := Cfg{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false, Retries: 3}
	m := &Manager{
		cache: make(map[string]*entry),
		ttl:   time.Minute,
		now:   time.Now,
		dial: func(ctx context.Context, cfg Cfg) (Client, error) {
			return nil, errBoom
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if _, err := m.DialContext(ctx, cfg, "tcp", "z:80"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > dialRetryDelay/2 {
		t.Fatalf("unexpected dial delay after cancel: %v", elapsed)
	}
}
