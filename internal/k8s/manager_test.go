package k8s

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

var errBoom = errors.New("boom")

func TestDialNonPersistentClosesSessionOnConnClose(t *testing.T) {
	m := NewManager()
	starts := atomic.Int32{}
	closes := atomic.Int32{}

	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		starts.Add(1)
		return &session{
			localAddr: "127.0.0.1:18080",
			closeFn: func() error {
				closes.Add(1)
				return nil
			},
		}, nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: false}
	conn, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if starts.Load() != 1 {
		t.Fatalf("expected 1 session start, got %d", starts.Load())
	}
	if closes.Load() != 1 {
		t.Fatalf("expected 1 session close, got %d", closes.Load())
	}
}

func TestDialPersistentCaches(t *testing.T) {
	m := NewManager()
	starts := atomic.Int32{}
	dials := atomic.Int32{}

	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		starts.Add(1)
		return stubSession(cfg, "127.0.0.1:18080"), nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn1.Close()
	_ = conn2.Close()

	if starts.Load() != 1 {
		t.Fatalf("expected 1 session start, got %d", starts.Load())
	}
	if dials.Load() != 2 {
		t.Fatalf("expected 2 local dials, got %d", dials.Load())
	}
}

func TestDialPersistentCoalescesConcurrentReconnects(t *testing.T) {
	m := NewManager()
	starts := atomic.Int32{}
	dials := atomic.Int32{}

	startReady := make(chan struct{})
	startRelease := make(chan struct{})

	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		if starts.Add(1) != 1 {
			return nil, errors.New("unexpected extra session start")
		}
		close(startReady)
		<-startRelease
		return stubSession(cfg, "127.0.0.1:18080"), nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}

	const workers = 5
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)

	for range workers {
		go func() {
			defer wg.Done()
			conn, err := m.DialContext(context.Background(), cfg, "tcp", "")
			if err != nil {
				errCh <- err
				return
			}
			_ = conn.Close()
		}()
	}

	select {
	case <-startReady:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for first start")
	}

	if starts.Load() != 1 {
		t.Fatalf("expected exactly one in-flight start, got %d", starts.Load())
	}
	close(startRelease)

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected concurrent dial error: %v", err)
		}
	}

	if starts.Load() != 1 {
		t.Fatalf("expected one shared start, got %d", starts.Load())
	}
	if dials.Load() != workers {
		t.Fatalf("expected %d local dials, got %d", workers, dials.Load())
	}
}

func TestDialPersistentReconnectsAfterSessionDone(t *testing.T) {
	m := NewManager()
	starts := atomic.Int32{}
	var s1 *session

	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		n := starts.Add(1)
		s := stubSession(cfg, "127.0.0.1:"+strconv.Itoa(18080+int(n)))
		if n == 1 {
			s1 = s
		}
		return s, nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	_ = conn1.Close()
	s1.finish(errBoom)

	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn2.Close()

	if starts.Load() != 2 {
		t.Fatalf("expected 2 starts after dead session, got %d", starts.Load())
	}
}

func TestDialPersistentReconnectsAfterDialFailure(t *testing.T) {
	m := NewManager()
	starts := atomic.Int32{}
	dials := atomic.Int32{}

	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		n := starts.Add(1)
		addr := "127.0.0.1:18081"
		if n > 1 {
			addr = "127.0.0.1:18082"
		}
		return stubSession(cfg, addr), nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		n := dials.Add(1)
		if n == 2 {
			return nil, errBoom
		}
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	_ = conn1.Close()

	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn2.Close()

	if starts.Load() != 2 {
		t.Fatalf("expected reconnect start after dial failure, got %d", starts.Load())
	}
}

func TestDialCachedUsesReplacedEntryWithoutReconnect(t *testing.T) {
	m := NewManager()
	t.Cleanup(func() { _ = m.Close() })

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	load, err := m.loadSettings()
	if err != nil {
		t.Fatalf("load cfg err: %v", err)
	}
	key := cacheKey(cfg, load)

	oldSes := stubSession(cfg, "127.0.0.1:18081")
	keepSes := stubSession(cfg, "127.0.0.1:18082")
	m.mu.Lock()
	m.cache[key] = &cacheEntry{ses: oldSes, lastUsed: m.now()}
	m.mu.Unlock()

	dialHit := make(chan struct{})
	dialCont := make(chan struct{})
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		switch address {
		case oldSes.localAddr:
			close(dialHit)
			<-dialCont
			return nil, errBoom
		case keepSes.localAddr:
			c1, c2 := net.Pipe()
			_ = c2.Close()
			return c1, nil
		default:
			return nil, errors.New("unexpected dial address")
		}
	}

	starts := atomic.Int32{}
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		starts.Add(1)
		return nil, errBoom
	}

	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "")
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	select {
	case <-dialHit:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for old cached dial")
	}

	m.mu.Lock()
	m.cache[key] = &cacheEntry{ses: keepSes, lastUsed: m.now()}
	m.mu.Unlock()
	close(dialCont)

	select {
	case err := <-errCh:
		t.Fatalf("expected dial success via replacement, got %v", err)
	case conn := <-connCh:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for replacement dial")
	}

	if starts.Load() != 0 {
		t.Fatalf("expected no reconnect attempt, got %d", starts.Load())
	}

	m.mu.Lock()
	cur := m.cache[key]
	m.mu.Unlock()
	if cur == nil || cur.ses != keepSes {
		t.Fatalf("expected replacement cached session to be preserved")
	}
}

func TestDialCachedKeepsReplacedEntryOnReconnectSuccess(t *testing.T) {
	m := NewManager()
	t.Cleanup(func() { _ = m.Close() })

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	load, err := m.loadSettings()
	if err != nil {
		t.Fatalf("load cfg err: %v", err)
	}
	key := cacheKey(cfg, load)

	oldSes := stubSession(cfg, "127.0.0.1:18081")
	keepSes := stubSession(cfg, "127.0.0.1:18082")
	reconnectSes := stubSession(cfg, "127.0.0.1:18083")
	reconnectClosed := atomic.Int32{}
	reconnectSes.closeFn = func() error {
		reconnectClosed.Add(1)
		reconnectSes.closed.Do(func() {
			close(reconnectSes.stopCh)
		})
		reconnectSes.finish(nil)
		return nil
	}

	m.mu.Lock()
	m.cache[key] = &cacheEntry{ses: oldSes, lastUsed: m.now()}
	m.mu.Unlock()

	dialHit := make(chan struct{})
	dialCont := make(chan struct{})
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		switch address {
		case oldSes.localAddr:
			close(dialHit)
			<-dialCont
			return nil, errBoom
		case reconnectSes.localAddr, keepSes.localAddr:
			c1, c2 := net.Pipe()
			_ = c2.Close()
			return c1, nil
		default:
			return nil, errors.New("unexpected dial address")
		}
	}

	starts := atomic.Int32{}
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		if starts.Add(1) > 1 {
			return nil, errors.New("unexpected reconnect attempt")
		}
		return reconnectSes, nil
	}

	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := m.DialContext(context.Background(), cfg, "tcp", "")
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	select {
	case <-dialHit:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for old cached dial")
	}

	m.mu.Lock()
	m.cache[key] = &cacheEntry{ses: keepSes, lastUsed: m.now()}
	m.mu.Unlock()
	close(dialCont)

	select {
	case err := <-errCh:
		t.Fatalf("expected successful dial via replacement, got err %v", err)
	case conn := <-connCh:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for dial completion")
	}

	if starts.Load() != 0 {
		t.Fatalf("expected no reconnect attempt, got %d", starts.Load())
	}
	if reconnectClosed.Load() != 0 {
		t.Fatalf("expected no reconnect session lifecycle when replacement exists")
	}

	m.mu.Lock()
	cur := m.cache[key]
	m.mu.Unlock()
	if cur == nil || cur.ses != keepSes {
		t.Fatalf("expected replacement cached session to be preserved")
	}
}

func TestDialRetry(t *testing.T) {
	m := NewManager()
	m.retryDelay = time.Millisecond
	starts := atomic.Int32{}
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		if starts.Add(1) < 2 {
			return nil, errBoom
		}
		return stubSession(cfg, "127.0.0.1:18080"), nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{
		Namespace: "default",
		Pod:       "api",
		Port:      8080,
		Persist:   false,
		Retries:   1,
	}
	conn, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if starts.Load() != 2 {
		t.Fatalf("expected 2 starts with retry, got %d", starts.Load())
	}
}

func TestDialRetryHonorsContextCancel(t *testing.T) {
	m := NewManager()
	m.retryDelay = time.Millisecond
	starts := atomic.Int32{}
	ctx, cancel := context.WithCancel(context.Background())
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		starts.Add(1)
		cancel()
		return nil, errBoom
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		t.Fatalf("dial should not be called on failed start")
		return nil, nil
	}

	cfg := Cfg{
		Namespace:  "default",
		TargetKind: targetKindPod,
		TargetName: "api",
		Port:       8080,
		Persist:    false,
		Retries:    3,
	}
	_, err := m.DialContext(ctx, cfg, "tcp", "")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if starts.Load() != 1 {
		t.Fatalf("expected one start before cancel, got %d", starts.Load())
	}
}

func TestDialPersistentCacheKeyIncludesTargetAndPortRef(t *testing.T) {
	m := NewManager()
	starts := atomic.Int32{}
	dials := atomic.Int32{}

	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		n := starts.Add(1)
		return stubSession(cfg, "127.0.0.1:"+strconv.Itoa(19080+int(n))), nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	base := Cfg{
		Namespace:  "default",
		TargetKind: targetKindService,
		TargetName: "api",
		PortName:   "http",
		Persist:    true,
	}
	conn1, err := m.DialContext(context.Background(), base, "tcp", "")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	conn2, err := m.DialContext(context.Background(), base, "tcp", "")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn1.Close()
	_ = conn2.Close()

	altPort := base
	altPort.PortName = "metrics"
	conn3, err := m.DialContext(context.Background(), altPort, "tcp", "")
	if err != nil {
		t.Fatalf("dial3 err: %v", err)
	}
	_ = conn3.Close()

	altTarget := base
	altTarget.TargetName = "api-canary"
	conn4, err := m.DialContext(context.Background(), altTarget, "tcp", "")
	if err != nil {
		t.Fatalf("dial4 err: %v", err)
	}
	_ = conn4.Close()

	if starts.Load() != 3 {
		t.Fatalf("expected 3 starts for distinct target/port refs, got %d", starts.Load())
	}
	if dials.Load() != 4 {
		t.Fatalf("expected 4 local dials, got %d", dials.Load())
	}
}

func TestDialRejectsUnsupportedNetwork(t *testing.T) {
	m := NewManager()
	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080}
	_, err := m.DialContext(context.Background(), cfg, "udp", "")
	if err == nil {
		t.Fatalf("expected unsupported network error")
	}
}

func TestDialNormalizesCfgBoundary(t *testing.T) {
	m := NewManager()
	var got Cfg
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		got = cfg
		return stubSession(cfg, "127.0.0.1:18080"), nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{
		Namespace: " default ",
		Pod:       " api ",
		Port:      8080,
		Container: " app ",
		Address:   " 127.0.0.1 ",
	}
	conn, err := m.DialContext(context.Background(), cfg, " tcp ", "")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if got.Namespace != "default" {
		t.Fatalf("expected normalized namespace, got %q", got.Namespace)
	}
	if got.Pod != "api" {
		t.Fatalf("expected normalized pod, got %q", got.Pod)
	}
	if got.Container != "app" {
		t.Fatalf("expected normalized container, got %q", got.Container)
	}
	if got.Address != "127.0.0.1" {
		t.Fatalf("expected normalized address, got %q", got.Address)
	}
}

func TestDialRejectsOutOfRangePort(t *testing.T) {
	m := NewManager()
	cfg := Cfg{Namespace: "default", Pod: "api", Port: 70000}
	_, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("expected out of range error, got %v", err)
	}
}

func TestCacheTTL(t *testing.T) {
	m := NewManager()
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	m.ttl = time.Minute

	starts := atomic.Int32{}
	closes := atomic.Int32{}
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		starts.Add(1)
		s := stubSession(cfg, "127.0.0.1:18080")
		s.closeFn = func() error {
			closes.Add(1)
			s.closed.Do(func() {
				close(s.stopCh)
			})
			s.finish(nil)
			return nil
		}
		return s, nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	conn1, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial1 err: %v", err)
	}
	_ = conn1.Close()

	now = now.Add(2 * time.Minute)
	conn2, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial2 err: %v", err)
	}
	_ = conn2.Close()

	if starts.Load() != 2 {
		t.Fatalf("expected stale cache reconnect, got %d starts", starts.Load())
	}
	if closes.Load() != 1 {
		t.Fatalf("expected stale cached session close, got %d", closes.Load())
	}
}

func TestManagerClose(t *testing.T) {
	m := NewManager()
	closes := atomic.Int32{}
	m.start = func(ctx context.Context, cfg Cfg, load loadSettings) (*session, error) {
		s := stubSession(cfg, "127.0.0.1:18080")
		s.closeFn = func() error {
			closes.Add(1)
			s.closed.Do(func() {
				close(s.stopCh)
			})
			s.finish(nil)
			return nil
		}
		return s, nil
	}
	m.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}

	cfg := Cfg{Namespace: "default", Pod: "api", Port: 8080, Persist: true}
	conn, err := m.DialContext(context.Background(), cfg, "tcp", "")
	if err != nil {
		t.Fatalf("dial err: %v", err)
	}
	_ = conn.Close()

	if err := m.Close(); err != nil {
		t.Fatalf("manager close err: %v", err)
	}
	if closes.Load() != 1 {
		t.Fatalf("expected cached session close on manager close, got %d", closes.Load())
	}
}

func TestSessionCloseHookStillClosesLifecycle(t *testing.T) {
	s := &session{
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	hookCalled := atomic.Int32{}
	s.closeFn = func() error {
		hookCalled.Add(1)
		return nil
	}
	go func() {
		<-s.stopCh
		s.finish(nil)
	}()

	if err := s.close(); err != nil {
		t.Fatalf("session close err: %v", err)
	}
	if hookCalled.Load() != 1 {
		t.Fatalf("expected close hook call count 1, got %d", hookCalled.Load())
	}
}

func TestShouldFallback(t *testing.T) {
	if shouldFallback(nil) {
		t.Fatalf("expected false for nil err")
	}
	if !shouldFallback(errors.New("websocket: bad handshake")) {
		t.Fatalf("expected websocket handshake to fallback")
	}
	if !shouldFallback(errors.New("proxy: unknown scheme: https")) {
		t.Fatalf("expected unknown scheme to fallback")
	}
	if shouldFallback(errors.New("forbidden")) {
		t.Fatalf("expected forbidden error to not force fallback")
	}
}

func TestWaitTargetPodHonorsContextCancel(t *testing.T) {
	cs := fake.NewClientset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := waitTargetPod(ctx, cs.AppsV1(), cs.CoreV1(), "default", Cfg{
		TargetKind: targetKindPod,
		TargetName: "api",
		PodWait:    time.Second,
	})
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got %v", err)
	}
}

func TestResolveForwardTargetServiceNamedPort(t *testing.T) {
	cs := fake.NewClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Ports: []corev1.ServicePort{
					{
						Name:       "web",
						Port:       80,
						TargetPort: intstr.FromString("http"),
					},
				},
			},
		},
		testPod("api-0", map[string]string{"app": "api"}, true, "api", "http", 8080),
	)

	cfg := Cfg{
		Namespace:  "default",
		TargetKind: targetKindService,
		TargetName: "api",
		PortName:   "web",
		PodWait:    0,
	}
	got, err := resolveForwardTarget(context.Background(), cs.AppsV1(), cs.CoreV1(), "default", cfg)
	if err != nil {
		t.Fatalf("resolve target err: %v", err)
	}
	if got.pod != "api-0" {
		t.Fatalf("expected api-0, got %q", got.pod)
	}
	if got.port != 8080 {
		t.Fatalf("expected remote port 8080, got %d", got.port)
	}
}

func TestResolveForwardTargetDeploymentPicksReadyPod(t *testing.T) {
	cs := fake.NewClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
		},
		testPod("api-2", map[string]string{"app": "api"}, false, "api", "http", 8080),
		testPod("api-1", map[string]string{"app": "api"}, true, "api", "http", 8080),
		testPodPending("api-0", map[string]string{"app": "api"}),
	)
	cfg := Cfg{
		Namespace:  "default",
		TargetKind: targetKindDeployment,
		TargetName: "api",
		Port:       8080,
		PodWait:    0,
	}

	got, err := resolveForwardTarget(context.Background(), cs.AppsV1(), cs.CoreV1(), "default", cfg)
	if err != nil {
		t.Fatalf("resolve target err: %v", err)
	}
	if got.pod != "api-1" {
		t.Fatalf("expected ready pod api-1, got %q", got.pod)
	}
	if got.port != 8080 {
		t.Fatalf("expected remote port 8080, got %d", got.port)
	}
}

func TestResolveForwardTargetStatefulSetDeterministicPod(t *testing.T) {
	cs := fake.NewClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
			},
		},
		testPod("db-1", map[string]string{"app": "db"}, true, "db", "pg", 5432),
		testPod("db-0", map[string]string{"app": "db"}, true, "db", "pg", 5432),
	)
	cfg := Cfg{
		Namespace:  "default",
		TargetKind: targetKindStatefulSet,
		TargetName: "db",
		PortName:   "pg",
		PodWait:    0,
	}

	got, err := resolveForwardTarget(context.Background(), cs.AppsV1(), cs.CoreV1(), "default", cfg)
	if err != nil {
		t.Fatalf("resolve target err: %v", err)
	}
	if got.pod != "db-0" {
		t.Fatalf("expected deterministic pod db-0, got %q", got.pod)
	}
	if got.port != 5432 {
		t.Fatalf("expected remote port 5432, got %d", got.port)
	}
}

func TestResolveForwardTargetServiceNamedPortAmbiguousAcrossContainers(t *testing.T) {
	p := testPod("api-0", map[string]string{"app": "api"}, true, "api", "http", 8080)
	p.Spec.Containers = append(p.Spec.Containers, corev1.Container{
		Name: "sidecar",
		Ports: []corev1.ContainerPort{
			{Name: "http", ContainerPort: 18080},
		},
	})

	cs := fake.NewClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
				Ports: []corev1.ServicePort{
					{
						Name:       "web",
						Port:       80,
						TargetPort: intstr.FromString("http"),
					},
				},
			},
		},
		p,
	)
	cfg := Cfg{
		Namespace:  "default",
		TargetKind: targetKindService,
		TargetName: "api",
		PortName:   "web",
		PodWait:    0,
	}

	_, err := resolveForwardTarget(context.Background(), cs.AppsV1(), cs.CoreV1(), "default", cfg)
	if err == nil || !strings.Contains(err.Error(), "ambiguous named port") {
		t.Fatalf("expected ambiguous named port error, got %v", err)
	}
}

func stubSession(cfg Cfg, addr string) *session {
	s := &session{
		localAddr: addr,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	go func() {
		<-s.stopCh
		s.finish(nil)
	}()
	return s
}

func testPod(
	name string,
	labels map[string]string,
	ready bool,
	cName, pName string,
	pNum int32,
) *corev1.Pod {
	conds := []corev1.PodCondition{{
		Type:   corev1.PodReady,
		Status: corev1.ConditionFalse,
	}}
	if ready {
		conds[0].Status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: cName,
					Ports: []corev1.ContainerPort{
						{Name: pName, ContainerPort: pNum},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: conds,
		},
	}
}

func testPodPending(name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}
}
