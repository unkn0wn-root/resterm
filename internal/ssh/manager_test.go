package ssh

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

type fakeClient struct {
	dials      atomic.Int32
	requests   atomic.Int32
	closed     atomic.Int32
	failDial   bool
	requestErr error
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
	if f.requestErr != nil {
		return false, nil, f.requestErr
	}
	return true, nil, nil
}

func (f *fakeClient) Close() error {
	f.closed.Add(1)
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

type blockingCloseClient struct {
	dials    atomic.Int32
	requests atomic.Int32
	closed   atomic.Int32

	failAfterFirst bool
	closeStarted   chan struct{}
	closeRelease   chan struct{}
	closeOnce      sync.Once
	releaseOnce    sync.Once
}

func newBlockingCloseClient(failAfterFirst bool) *blockingCloseClient {
	return &blockingCloseClient{
		failAfterFirst: failAfterFirst,
		closeStarted:   make(chan struct{}),
		closeRelease:   make(chan struct{}),
	}
}

func (b *blockingCloseClient) Dial(network, addr string) (net.Conn, error) {
	idx := b.dials.Add(1)
	if b.failAfterFirst && idx > 1 {
		return nil, errBoom
	}
	c1, c2 := net.Pipe()
	_ = c2.Close()
	return c1, nil
}

func (b *blockingCloseClient) SendRequest(
	name string,
	wantReply bool,
	payload []byte,
) (bool, []byte, error) {
	b.requests.Add(1)
	return true, nil, nil
}

func (b *blockingCloseClient) Close() error {
	b.closed.Add(1)
	b.closeOnce.Do(func() {
		close(b.closeStarted)
	})
	<-b.closeRelease
	return nil
}

func (b *blockingCloseClient) release() {
	b.releaseOnce.Do(func() {
		close(b.closeRelease)
	})
}

func newTestManager(dial func(context.Context, execConfig) (sshClient, error)) *Manager {
	return &Manager{
		cache:  newSessionCache(time.Minute, time.Now),
		opener: newSessionOpener(dial, time.Millisecond),
	}
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("condition not reached before timeout")
}

func waitForCloseStarted(t *testing.T, cli *blockingCloseClient) {
	t.Helper()
	select {
	case <-cli.closeStarted:
	case <-time.After(time.Second):
		t.Fatalf("close did not start")
	}
}

func assertDialFinishes(t *testing.T, m *Manager, cfg Config) {
	t.Helper()
	errCh := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
		if conn != nil {
			_ = conn.Close()
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("dial err: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("dial blocked on manager lock")
	}
}

func keyForTest(t *testing.T, cfg Config) sessionKey {
	t.Helper()
	execCfg, err := prepareExecConfig(cfg)
	if err != nil {
		t.Fatalf("prepare config: %v", err)
	}
	return execCfg.key
}

func TestDialNonPersistent(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false}
	dials := atomic.Int32{}

	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		dials.Add(1)
		return &fakeClient{}, nil
	})

	conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if got := dials.Load(); got != 1 {
		t.Fatalf("expected 1 dial, got %d", got)
	}
}

func TestDialNonPersistentClosesSessionWhenManagerClosesDuringConnect(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false}
	started := make(chan struct{})
	release := make(chan struct{})
	fc := &fakeClient{}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		close(started)
		<-release
		return fc, nil
	})

	errCh := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
		if conn != nil {
			_ = conn.Close()
		}
		errCh <- err
	}()

	<-started
	if err := m.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	close(release)

	err := <-errCh
	if !errors.Is(err, errManagerClosed) {
		t.Fatalf("expected closed manager error, got %v", err)
	}
	if got := fc.closed.Load(); got != 1 {
		t.Fatalf("expected connected session to be closed, got %d", got)
	}
	if got := fc.dials.Load(); got != 0 {
		t.Fatalf("expected target dial not to run after close, got %d", got)
	}
}

func TestDialPersistentCaches(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true, KeepAlive: 0}
	dials := atomic.Int32{}

	fc := &fakeClient{}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		dials.Add(1)
		return fc, nil
	})

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
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true, KeepAlive: 0}
	first := &scriptedClient{results: []error{nil, errBoom}}
	second := &scriptedClient{}
	dials := atomic.Int32{}

	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		switch dials.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			return nil, errors.New("unexpected dial")
		}
	})

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

	key := keyForTest(t, cfg)
	m.cache.mu.Lock()
	ent := m.cache.entries[key]
	m.cache.mu.Unlock()
	if ent == nil || ent.ses.cli != second {
		t.Fatalf("expected cache to hold replacement client")
	}
}

func TestEvictCachedSessionClosesOutsideManagerLock(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true}
	otherCfg := Config{Host: "other", Port: 22, User: "u", Pass: "p", Persist: false}
	blocking := newBlockingCloseClient(true)
	defer blocking.release()

	sshDials := atomic.Int32{}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		if cfg.Host == "h" && sshDials.Add(1) == 1 {
			return blocking, nil
		}
		return &fakeClient{}, nil
	})

	conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if err != nil {
		t.Fatalf("seed dial err: %v", err)
	}
	_ = conn.Close()

	errCh := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:81")
		if conn != nil {
			_ = conn.Close()
		}
		errCh <- err
	}()

	waitForCloseStarted(t, blocking)
	assertDialFinishes(t, m, otherCfg)

	blocking.release()
	if err := <-errCh; err != nil {
		t.Fatalf("reconnect dial err: %v", err)
	}
}

func TestPurgeClosesStaleSessionOutsideManagerLock(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true}
	otherCfg := Config{Host: "other", Port: 22, User: "u", Pass: "p", Persist: false}
	blocking := newBlockingCloseClient(false)
	defer blocking.release()

	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		return &fakeClient{}, nil
	})
	m.cache.entries[keyForTest(t, cfg)] = newCacheEntry(
		newSession(blocking, 0),
		time.Now().Add(-2*time.Minute),
	)

	errCh := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
		if conn != nil {
			_ = conn.Close()
		}
		errCh <- err
	}()

	waitForCloseStarted(t, blocking)
	assertDialFinishes(t, m, otherCfg)

	blocking.release()
	if err := <-errCh; err != nil {
		t.Fatalf("purge dial err: %v", err)
	}
}

func TestDialPersistentConcurrentSharesClient(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true}
	dials := atomic.Int32{}
	start := make(chan struct{})
	fc := &fakeClient{}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		dials.Add(1)
		<-start
		return fc, nil
	})

	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	conns := make(chan net.Conn, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
			if err != nil {
				errs <- err
				return
			}
			conns <- conn
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(conns)

	for err := range errs {
		t.Fatalf("dial err: %v", err)
	}
	for conn := range conns {
		_ = conn.Close()
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("expected 1 ssh client dial, got %d", got)
	}
	if got := fc.dials.Load(); got != n {
		t.Fatalf("expected %d forwarded dials, got %d", n, got)
	}
}

func TestCloseUnblocksPersistentInflightWaiters(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true}
	started := make(chan struct{})
	release := make(chan struct{})
	fc := &fakeClient{}
	dials := atomic.Int32{}
	startOnce := sync.Once{}

	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		if dials.Add(1) > 1 {
			return nil, errors.New("unexpected dial")
		}
		startOnce.Do(func() { close(started) })
		<-release
		return fc, nil
	})

	err1 := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
		if conn != nil {
			_ = conn.Close()
		}
		err1 <- err
	}()

	<-started

	err2 := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:81")
		if conn != nil {
			_ = conn.Close()
		}
		err2 <- err
	}()

	if err := m.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}

	select {
	case err := <-err2:
		if !errors.Is(err, errManagerClosed) {
			t.Fatalf("expected waiter closed error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("waiter remained blocked after manager close")
	}

	close(release)
	err := <-err1
	if !errors.Is(err, errManagerClosed) {
		t.Fatalf("expected opener closed error, got %v", err)
	}
	if got := fc.closed.Load(); got != 1 {
		t.Fatalf("expected connected session to close once, got %d", got)
	}
}

func TestDialRetry(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false, Retries: 2}
	count := atomic.Int32{}

	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		if count.Add(1); count.Load() < 2 {
			return nil, errBoom
		}
		return &fakeClient{}, nil
	})

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
	cfg := Config{
		Host:      "h",
		Port:      22,
		User:      "u",
		Pass:      "p",
		Persist:   true,
		KeepAlive: 5 * time.Millisecond,
	}
	fc := &fakeClient{}

	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		return fc, nil
	})

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

func TestKeepAliveFailureReconnects(t *testing.T) {
	cfg := Config{
		Host:      "h",
		Port:      22,
		User:      "u",
		Pass:      "p",
		Persist:   true,
		KeepAlive: time.Millisecond,
	}
	first := &fakeClient{requestErr: errBoom}
	second := &fakeClient{}
	dials := atomic.Int32{}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		switch dials.Add(1) {
		case 1:
			return first, nil
		case 2:
			return second, nil
		default:
			return nil, errors.New("unexpected dial")
		}
	})

	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "k:80")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	_ = conn1.Close()

	waitUntil(t, func() bool { return first.closed.Load() > 0 })

	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "k:81")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn2.Close()

	if got := dials.Load(); got != 2 {
		t.Fatalf("expected reconnect after keepalive failure, got %d dials", got)
	}
	if got := second.dials.Load(); got != 1 {
		t.Fatalf("expected replacement session dial, got %d", got)
	}
}

func TestPersistentTargetDialFailureClosesSession(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true}
	fc := &fakeClient{failDial: true}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		return fc, nil
	})

	_, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected target dial error, got %v", err)
	}
	if got := fc.closed.Load(); got != 1 {
		t.Fatalf("expected failed session to close once, got %d", got)
	}

	key := keyForTest(t, cfg)
	m.cache.mu.Lock()
	ent := m.cache.entries[key]
	m.cache.mu.Unlock()
	if ent != nil {
		t.Fatalf("expected failed session to be removed from cache")
	}
}

func TestManagerCloseIdempotent(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: true}
	fc := &fakeClient{}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		return fc, nil
	})

	conn, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if err := m.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second close err: %v", err)
	}
	if got := fc.closed.Load(); got != 1 {
		t.Fatalf("expected cached client to close once, got %d", got)
	}
}

func TestClosedManagerRejectsDial(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p"}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		return &fakeClient{}, nil
	})
	if err := m.Close(); err != nil {
		t.Fatalf("close err: %v", err)
	}

	_, err := m.DialContext(context.Background(), cfg, "tcp", "x:80")
	if !errors.Is(err, errManagerClosed) {
		t.Fatalf("expected closed manager error, got %v", err)
	}
}

func TestDialRetryHonorsCancelledContext(t *testing.T) {
	cfg := Config{Host: "h", Port: 22, User: "u", Pass: "p", Persist: false, Retries: 3}
	m := newTestManager(func(ctx context.Context, cfg execConfig) (sshClient, error) {
		return nil, errBoom
	})

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
