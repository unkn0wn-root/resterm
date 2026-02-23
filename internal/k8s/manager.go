package k8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/connutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	spdytransport "k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/flowcontrol"
)

const (
	defaultDialRetryDelay   = 150 * time.Millisecond
	defaultLocalDialTimeout = 10 * time.Second
	closeWaitWindow         = 3 * time.Second
	podPollInterval         = 300 * time.Millisecond
)

type startFn func(context.Context, Cfg, loadCfg) (*session, error)
type dialFn func(context.Context, string, string) (net.Conn, error)

type Manager struct {
	mu    sync.Mutex
	cache map[string]*entry
	// In-flight session starts keyed by cache key. Waiters block on the channel
	// to avoid duplicate reconnect work under contention.
	inflight map[string]chan struct{}
	ttl      time.Duration
	now      func() time.Time
	opt      LoadOpt
	start    startFn
	dial     dialFn
	// Test-tunable retry delay for reconnect attempts.
	retryDelay time.Duration
}

type entry struct {
	cfg      Cfg
	ses      *session
	lastUsed time.Time
}

type session struct {
	cfg       Cfg
	localAddr string
	stopCh    chan struct{}
	doneCh    chan struct{}

	mu       sync.RWMutex
	err      error
	closed   sync.Once
	finished sync.Once
	closeFn  func() error
}

type fwdTarget struct {
	pod  string
	port int
}

type targetPod struct {
	pod *corev1.Pod
	svc *corev1.Service
}

func NewManager() *Manager {
	dialer := &net.Dialer{Timeout: defaultLocalDialTimeout}
	return &Manager{
		cache:      make(map[string]*entry),
		inflight:   make(map[string]chan struct{}),
		ttl:        defaultTTL,
		now:        time.Now,
		start:      startSession,
		dial:       dialer.DialContext,
		retryDelay: defaultDialRetryDelay,
	}
}

func (m *Manager) SetLoadOptions(opt LoadOpt) {
	if m == nil {
		return
	}
	opt.ExecAllowlist = append([]string(nil), opt.ExecAllowlist...)
	m.mu.Lock()
	m.opt = opt
	m.mu.Unlock()
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for key, ent := range m.cache {
		if err := ent.ses.close(); err != nil {
			errs = append(errs, err)
		}
		delete(m.cache, key)
	}
	for key, ch := range m.inflight {
		close(ch)
		delete(m.inflight, key)
	}
	return errors.Join(errs...)
}

func (m *Manager) DialContext(
	ctx context.Context,
	cfg Cfg,
	network, addr string,
) (net.Conn, error) {
	// The target address argument is intentionally ignored for k8s:
	// traffic always goes through the active port-forward session.
	_ = addr
	cfg = normalizeCfg(cfg)
	if cfg.Namespace == "" {
		cfg.Namespace = defaultNamespace
	}

	if cfg.targetRef() == "" {
		return nil, errors.New("k8s: target required")
	}
	if cfg.Port <= 0 && cfg.portRef() == "" {
		return nil, errors.New("k8s: port required")
	}
	if cfg.Port > 65535 {
		return nil, errors.New("k8s: port out of range")
	}

	load, err := m.loadCfg()
	if err != nil {
		return nil, err
	}

	if !cfg.Persist {
		return m.dialOnce(ctx, cfg, load, network)
	}
	return m.dialCached(ctx, cfg, load, network)
}

func (m *Manager) dialOnce(
	ctx context.Context,
	cfg Cfg,
	load loadCfg,
	network string,
) (net.Conn, error) {
	ses, err := m.connect(ctx, cfg, load)
	if err != nil {
		return nil, err
	}

	conn, err := m.dialSession(ctx, ses, network)
	if err != nil {
		return nil, joinCleanupErr(err, ses.close())
	}
	return connutil.WrapConn(conn, ses.close), nil
}

func (m *Manager) dialCached(
	ctx context.Context,
	cfg Cfg,
	load loadCfg,
	network string,
) (net.Conn, error) {
	key := cacheKey(cfg, load)

	for {
		m.mu.Lock()
		m.purgeLocked()
		ent := m.cache[key]
		if ent != nil {
			ent.lastUsed = m.now()
			ses := ent.ses
			m.mu.Unlock()

			if ses != nil && ses.alive() {
				conn, err := m.dialSession(ctx, ses, network)
				if err == nil {
					return conn, nil
				}
			}

			m.mu.Lock()
			if cur := m.cache[key]; cur == ent {
				_ = ent.ses.close()
				delete(m.cache, key)
			} else {
				_ = ent.ses.close()
			}
			m.mu.Unlock()
			continue
		}

		waitCh, waiting := m.inflight[key]
		if waiting {
			m.mu.Unlock()
			select {
			case <-waitCh:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		token := make(chan struct{})
		m.inflight[key] = token
		m.mu.Unlock()

		ses, err := m.connect(ctx, cfg, load)
		if err != nil {
			m.releaseInflight(key, token)
			return nil, err
		}

		m.mu.Lock()
		if cur := m.cache[key]; cur != nil && cur.ses != nil && cur.ses.alive() {
			m.mu.Unlock()
			_ = ses.close()
			m.releaseInflight(key, token)

			conn, dialErr := m.dialSession(ctx, cur.ses, network)
			if dialErr == nil {
				return conn, nil
			}

			m.mu.Lock()
			if latest := m.cache[key]; latest == cur {
				_ = cur.ses.close()
				delete(m.cache, key)
			} else {
				_ = cur.ses.close()
			}
			m.mu.Unlock()
			continue
		}

		m.cache[key] = &entry{cfg: cfg, ses: ses, lastUsed: m.now()}
		m.mu.Unlock()
		m.releaseInflight(key, token)

		conn, err := m.dialSession(ctx, ses, network)
		if err == nil {
			return conn, nil
		}

		m.mu.Lock()
		if cur := m.cache[key]; cur != nil && cur.ses == ses {
			delete(m.cache, key)
		}
		m.mu.Unlock()
		return nil, joinCleanupErr(err, ses.close())
	}
}

func (m *Manager) releaseInflight(key string, token chan struct{}) {
	if m == nil || token == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.inflight[key]; ok && cur == token {
		delete(m.inflight, key)
		close(token)
	}
}

func joinCleanupErr(baseErr error, cleanupErr error) error {
	if cleanupErr == nil {
		return baseErr
	}
	if baseErr == nil {
		return cleanupErr
	}
	return errors.Join(baseErr, cleanupErr)
}

func (m *Manager) connect(ctx context.Context, cfg Cfg, load loadCfg) (*session, error) {
	if m == nil || m.start == nil {
		return nil, errors.New("k8s: manager unavailable")
	}
	attempts := cfg.Retries + 1
	// Defensive for manually built Cfg values that bypass NormalizeProfile.
	if attempts < 1 {
		attempts = 1
	}
	retryDelay := m.retryDelay
	if retryDelay <= 0 {
		retryDelay = defaultDialRetryDelay
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		ses, err := m.start(ctx, cfg, load)
		if err == nil {
			return ses, nil
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		if i+1 < attempts {
			if err := connutil.WaitWithContext(ctx, retryDelay); err != nil {
				return nil, err
			}
		}
	}
	if lastErr == nil {
		lastErr = errors.New("k8s: port-forward start failed")
	}
	return nil, lastErr
}

func (m *Manager) dialSession(ctx context.Context, ses *session, network string) (net.Conn, error) {
	if m == nil || m.dial == nil {
		return nil, errors.New("k8s: local dialer unavailable")
	}
	n, err := normalizeNetwork(network)
	if err != nil {
		return nil, err
	}
	addr := ses.localAddr
	if addr == "" {
		return nil, errors.New("k8s: local forward address unavailable")
	}
	return m.dial(ctx, n, addr)
}

func (m *Manager) purgeLocked() {
	now := m.now()
	for key, ent := range m.cache {
		// Defensive: keep cache healthy even if malformed entries are injected in tests.
		if ent == nil || ent.ses == nil {
			delete(m.cache, key)
			continue
		}
		expired := now.Sub(ent.lastUsed) > m.ttl
		dead := !ent.ses.alive()
		if !expired && !dead {
			continue
		}
		_ = ent.ses.close()
		delete(m.cache, key)
	}
}

func (m *Manager) loadCfg() (loadCfg, error) {
	if m == nil {
		return loadCfg{}, errors.New("k8s: manager unavailable")
	}
	m.mu.Lock()
	opt := m.opt
	m.mu.Unlock()
	opt.ExecAllowlist = append([]string(nil), opt.ExecAllowlist...)
	return normalizeLoadOpt(opt)
}

func (s *session) alive() bool {
	if s == nil || s.doneCh == nil {
		return false
	}
	select {
	case <-s.doneCh:
		return false
	default:
		return true
	}
}

func (s *session) finish(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.err = err
	s.mu.Unlock()

	s.finished.Do(func() {
		if s.doneCh != nil {
			close(s.doneCh)
		}
	})
}

func (s *session) close() error {
	if s == nil {
		return nil
	}

	s.closed.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
	})

	var errs []error
	if s.closeFn != nil {
		if err := s.closeFn(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.doneCh != nil {
		select {
		case <-s.doneCh:
		case <-time.After(closeWaitWindow):
			errs = append(errs, errors.New("k8s: timeout closing port-forward"))
		}
	}
	return errors.Join(errs...)
}

func (s *session) errValue() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.err
}

func startSession(ctx context.Context, cfg Cfg, load loadCfg) (*session, error) {
	ns := cfg.Namespace
	if ns == "" {
		return nil, errors.New("k8s: namespace required")
	}

	restCfg, err := RESTConfig(cfg, loadOptFromCfg(load))
	if err != nil {
		return nil, err
	}

	appsClient, coreClient, err := newTypedClients(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s: build client: %w", err)
	}
	rt, err := resolveForwardTarget(ctx, appsClient, coreClient, ns, cfg)
	if err != nil {
		return nil, err
	}

	u := coreClient.
		RESTClient().
		Post().
		Resource("pods").
		Namespace(ns).
		Name(rt.pod).
		SubResource("portforward").
		URL()
	dialer, err := buildDialer(u, restCfg)
	if err != nil {
		return nil, err
	}

	addresses := bindAddrs(cfg.Address)
	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	pf, err := portforward.NewOnAddresses(
		dialer,
		addresses,
		[]string{formatPortSpec(cfg.LocalPort, rt.port)},
		stopCh,
		readyCh,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		return nil, fmt.Errorf("k8s: build port-forwarder: %w", err)
	}

	ses := &session{
		cfg:    cfg,
		stopCh: stopCh,
		doneCh: make(chan struct{}),
	}
	go func() {
		ses.finish(pf.ForwardPorts())
	}()

	select {
	case <-readyCh:
	case <-ctx.Done():
		return nil, joinCleanupErr(ctx.Err(), ses.close())
	case <-ses.doneCh:
		if err := ses.errValue(); err != nil {
			return nil, err
		}
		return nil, errors.New("k8s: port-forward stopped before ready")
	}

	ports, err := pf.GetPorts()
	if err != nil {
		baseErr := fmt.Errorf("k8s: resolve forwarded ports: %w", err)
		return nil, joinCleanupErr(baseErr, ses.close())
	}
	if len(ports) == 0 {
		baseErr := errors.New("k8s: port-forward did not expose local ports")
		return nil, joinCleanupErr(baseErr, ses.close())
	}

	host := dialHost(addresses)
	ses.localAddr = net.JoinHostPort(host, strconv.Itoa(int(ports[0].Local)))
	return ses, nil
}

func newTypedClients(cfg *rest.Config) (
	appsv1client.AppsV1Interface,
	corev1client.CoreV1Interface,
	error,
) {
	if cfg == nil {
		return nil, nil, errors.New("missing rest config")
	}

	configShallowCopy := *cfg
	if configShallowCopy.UserAgent == "" {
		configShallowCopy.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	httpClient, err := rest.HTTPClientFor(&configShallowCopy)
	if err != nil {
		return nil, nil, err
	}
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		if configShallowCopy.Burst <= 0 {
			return nil, nil, fmt.Errorf(
				"burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0",
			)
		}
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(
			configShallowCopy.QPS,
			configShallowCopy.Burst,
		)
	}

	appsClient, err := appsv1client.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, nil, err
	}
	coreClient, err := corev1client.NewForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, nil, err
	}
	return appsClient, coreClient, nil
}

func resolveForwardTarget(
	ctx context.Context,
	apps appsv1client.AppsV1Interface,
	core corev1client.CoreV1Interface,
	ns string,
	cfg Cfg,
) (fwdTarget, error) {
	sel, err := waitTargetPod(ctx, apps, core, ns, cfg)
	if err != nil {
		return fwdTarget{}, err
	}
	rp, err := resolveRemotePort(cfg, sel.pod, sel.svc)
	if err != nil {
		return fwdTarget{}, err
	}
	return fwdTarget{pod: sel.pod.Name, port: rp}, nil
}

func waitTargetPod(
	ctx context.Context,
	apps appsv1client.AppsV1Interface,
	core corev1client.CoreV1Interface,
	namespace string,
	cfg Cfg,
) (targetPod, error) {
	if core == nil {
		return targetPod{}, errors.New("k8s: client unavailable")
	}
	ns := namespace
	if ns == "" {
		return targetPod{}, errors.New("k8s: namespace is required")
	}
	k, n := cfg.target()
	if k == "" {
		return targetPod{}, errors.New("k8s: target kind is required")
	}
	if n == "" {
		return targetPod{}, errors.New("k8s: target name is required")
	}
	t := cfg.PodWait
	id := targetID(k, n)

	var out targetPod
	check := func(ctx context.Context) (bool, error) {
		sel, err := selectTargetPod(ctx, apps, core, ns, k, n)
		if err != nil {
			return false, err
		}
		if sel.pod == nil {
			return false, nil
		}
		switch sel.pod.Status.Phase {
		case corev1.PodRunning:
			out = sel
			return true, nil
		case corev1.PodFailed, corev1.PodSucceeded:
			if k != targetKindPod {
				return false, nil
			}
			return false, fmt.Errorf(
				"k8s: pod %s/%s is %s",
				ns,
				sel.pod.Name,
				strings.ToLower(string(sel.pod.Status.Phase)),
			)
		default:
			return false, nil
		}
	}

	if t <= 0 {
		ok, err := check(ctx)
		if err != nil {
			return targetPod{}, fmt.Errorf("k8s: check target %s/%s: %w", ns, id, err)
		}
		if !ok {
			return targetPod{}, fmt.Errorf("k8s: target %s/%s has no running pods", ns, id)
		}
		return out, nil
	}

	err := wait.PollUntilContextTimeout(ctx, podPollInterval, t, true, check)
	if err != nil {
		return targetPod{}, fmt.Errorf("k8s: wait target %s/%s running: %w", ns, id, err)
	}
	return out, nil
}

func selectTargetPod(
	ctx context.Context,
	apps appsv1client.AppsV1Interface,
	core corev1client.CoreV1Interface,
	ns string,
	k TargetKind,
	name string,
) (targetPod, error) {
	switch k {
	case targetKindPod:
		p, err := core.Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return targetPod{}, nil
			}
			return targetPod{}, err
		}
		return targetPod{pod: p}, nil

	case targetKindService:
		svc, err := core.Services(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return targetPod{}, nil
			}
			return targetPod{}, err
		}
		ps, err := podsForService(ctx, core, ns, svc)
		if err != nil {
			return targetPod{}, err
		}
		p := pickPod(ps)
		return targetPod{pod: p, svc: svc}, nil

	case targetKindDeployment:
		if apps == nil {
			return targetPod{}, errors.New("k8s: client unavailable")
		}
		d, err := apps.Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return targetPod{}, nil
			}
			return targetPod{}, err
		}
		return targetBySelector(ctx, core, ns, d.Spec.Selector, "deployment", ns, d.Name)

	case targetKindStatefulSet:
		if apps == nil {
			return targetPod{}, errors.New("k8s: client unavailable")
		}
		s, err := apps.StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return targetPod{}, nil
			}
			return targetPod{}, err
		}
		return targetBySelector(ctx, core, ns, s.Spec.Selector, "statefulset", ns, s.Name)

	default:
		return targetPod{}, fmt.Errorf("k8s: unsupported target kind %q", k)
	}
}

func targetBySelector(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	ns string,
	sel *metav1.LabelSelector,
	kind, objNS, objName string,
) (targetPod, error) {
	ps, err := podsForLabelSelector(ctx, core, ns, sel, kind, objNS, objName)
	if err != nil {
		return targetPod{}, err
	}
	return targetPod{pod: pickPod(ps)}, nil
}

func podsForService(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	ns string,
	svc *corev1.Service,
) ([]corev1.Pod, error) {
	if svc == nil {
		return nil, errors.New("k8s: service is required")
	}
	if len(svc.Spec.Selector) == 0 {
		return nil, fmt.Errorf("k8s: service %s/%s has no selector", ns, svc.Name)
	}
	sel := labels.SelectorFromSet(svc.Spec.Selector)
	return listPods(ctx, core, ns, sel.String())
}

func podsForLabelSelector(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	ns string,
	sel *metav1.LabelSelector,
	kind, objNS, objName string,
) ([]corev1.Pod, error) {
	if sel == nil {
		return nil, fmt.Errorf("k8s: %s %s/%s has no selector", kind, objNS, objName)
	}
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return nil, fmt.Errorf("k8s: %s %s/%s selector: %w", kind, objNS, objName, err)
	}
	if s.Empty() {
		return nil, fmt.Errorf("k8s: %s %s/%s has empty selector", kind, objNS, objName)
	}
	return listPods(ctx, core, ns, s.String())
}

func listPods(
	ctx context.Context,
	core corev1client.CoreV1Interface,
	ns string,
	sel string,
) ([]corev1.Pod, error) {
	pods, err := core.Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	if pods == nil || len(pods.Items) == 0 {
		return nil, nil
	}
	return pods.Items, nil
}

func pickPod(pods []corev1.Pod) *corev1.Pod {
	if len(pods) == 0 {
		return nil
	}
	active := make([]corev1.Pod, 0, len(pods))
	for _, p := range pods {
		if p.DeletionTimestamp != nil {
			continue
		}
		active = append(active, p)
	}
	if len(active) == 0 {
		return nil
	}

	slices.SortFunc(active, func(a, b corev1.Pod) int {
		ar, br := podRank(a), podRank(b)
		if ar < br {
			return -1
		}
		if ar > br {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	out := active[0]
	return &out
}

func podRank(p corev1.Pod) int {
	const (
		rankRunningReady = iota
		rankRunningNotReady
		rankPending
		rankUnknown
		rankOther
	)
	switch p.Status.Phase {
	case corev1.PodRunning:
		if podReady(p.Status.Conditions) {
			return rankRunningReady
		}
		return rankRunningNotReady
	case corev1.PodPending:
		return rankPending
	case corev1.PodUnknown:
		return rankUnknown
	default:
		return rankOther
	}
}

func podReady(conds []corev1.PodCondition) bool {
	for _, c := range conds {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func targetID(k TargetKind, n string) string {
	return string(k) + "/" + n
}

func resolveRemotePort(cfg Cfg, pod *corev1.Pod, svc *corev1.Service) (int, error) {
	if pod == nil {
		return 0, errors.New("k8s: pod is required")
	}
	if svc == nil {
		return resolvePodPort(cfg, pod)
	}
	return resolveServicePort(cfg, pod, svc)
}

func resolvePodPort(cfg Cfg, pod *corev1.Pod) (int, error) {
	if cfg.Port > 0 {
		return cfg.Port, nil
	}
	return podPortByName(pod, cfg.Container, cfg.PortName)
}

func resolveServicePort(cfg Cfg, pod *corev1.Pod, svc *corev1.Service) (int, error) {
	sp, err := servicePortByCfg(cfg, svc)
	if err != nil {
		return 0, err
	}
	return serviceTargetPort(sp, pod, cfg.Container)
}

func servicePortByCfg(cfg Cfg, svc *corev1.Service) (corev1.ServicePort, error) {
	if svc == nil {
		return corev1.ServicePort{}, errors.New("k8s: service is required")
	}
	if len(svc.Spec.Ports) == 0 {
		return corev1.ServicePort{}, fmt.Errorf(
			"k8s: service %s/%s has no ports",
			svc.Namespace,
			svc.Name,
		)
	}
	if cfg.Port > 0 {
		var out []corev1.ServicePort
		for _, sp := range svc.Spec.Ports {
			if int(sp.Port) == cfg.Port {
				out = append(out, sp)
			}
		}
		return pickServicePort(out, svc, strconv.Itoa(cfg.Port))
	}

	name := cfg.PortName
	if name == "" {
		return corev1.ServicePort{}, errors.New("k8s: port is required")
	}

	var byName []corev1.ServicePort
	for _, sp := range svc.Spec.Ports {
		if strings.TrimSpace(sp.Name) == name {
			byName = append(byName, sp)
		}
	}
	if len(byName) > 0 {
		return pickServicePort(byName, svc, name)
	}

	var byTarget []corev1.ServicePort
	for _, sp := range svc.Spec.Ports {
		if sp.TargetPort.Type == intstr.String && strings.TrimSpace(sp.TargetPort.StrVal) == name {
			byTarget = append(byTarget, sp)
		}
	}
	return pickServicePort(byTarget, svc, name)
}

func pickServicePort(
	ports []corev1.ServicePort,
	svc *corev1.Service,
	ref string,
) (corev1.ServicePort, error) {
	switch len(ports) {
	case 0:
		return corev1.ServicePort{}, fmt.Errorf(
			"k8s: service %s/%s does not expose port %q",
			svc.Namespace,
			svc.Name,
			ref,
		)
	case 1:
		return ports[0], nil
	default:
		return corev1.ServicePort{}, fmt.Errorf(
			"k8s: service %s/%s has multiple ports matching %q",
			svc.Namespace,
			svc.Name,
			ref,
		)
	}
}

func serviceTargetPort(sp corev1.ServicePort, pod *corev1.Pod, cName string) (int, error) {
	switch sp.TargetPort.Type {
	case intstr.Int:
		if sp.TargetPort.IntVal > 0 {
			return int(sp.TargetPort.IntVal), nil
		}
	case intstr.String:
		if v := strings.TrimSpace(sp.TargetPort.StrVal); v != "" {
			return podPortByName(pod, cName, v)
		}
	}
	if sp.Port <= 0 {
		return 0, fmt.Errorf("k8s: service port %q is invalid", sp.Name)
	}
	return int(sp.Port), nil
}

func podPortByName(pod *corev1.Pod, cName, pName string) (int, error) {
	if pod == nil {
		return 0, errors.New("k8s: pod is required")
	}
	containerName := cName
	name := pName
	if name == "" {
		return 0, errors.New("k8s: port is required")
	}

	cs, err := pickContainers(pod, containerName)
	if err != nil {
		return 0, err
	}

	type hit struct {
		c string
		p int32
	}
	var hits []hit
	for _, c := range cs {
		for _, cp := range c.Ports {
			if strings.TrimSpace(cp.Name) != name {
				continue
			}
			if cp.ContainerPort <= 0 || cp.ContainerPort > 65535 {
				continue
			}
			hits = append(hits, hit{c: c.Name, p: cp.ContainerPort})
		}
	}
	if len(hits) == 0 {
		return 0, fmt.Errorf(
			"k8s: pod %s/%s does not expose named port %q",
			pod.Namespace,
			pod.Name,
			name,
		)
	}
	if containerName == "" && len(hits) > 1 {
		return 0, fmt.Errorf(
			"k8s: pod %s/%s has ambiguous named port %q across containers",
			pod.Namespace,
			pod.Name,
			name,
		)
	}
	return int(hits[0].p), nil
}

func pickContainers(pod *corev1.Pod, cName string) ([]corev1.Container, error) {
	if pod == nil {
		return nil, errors.New("k8s: pod is required")
	}
	name := cName
	if name == "" {
		return pod.Spec.Containers, nil
	}
	for _, c := range pod.Spec.Containers {
		if c.Name == name {
			return []corev1.Container{c}, nil
		}
	}
	return nil, fmt.Errorf(
		"k8s: pod %s/%s does not contain container %q",
		pod.Namespace,
		pod.Name,
		name,
	)
}

func buildDialer(u *url.URL, cfg *rest.Config) (httpstream.Dialer, error) {
	if u == nil {
		return nil, errors.New("k8s: port-forward url required")
	}
	if cfg == nil {
		return nil, errors.New("k8s: rest config required")
	}

	rt, upgrader, err := spdytransport.RoundTripperFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s: create spdy roundtripper: %w", err)
	}
	spdyDialer := spdytransport.NewDialer(
		upgrader,
		&http.Client{Transport: rt},
		http.MethodPost,
		u,
	)

	wsDialer, err := portforward.NewSPDYOverWebsocketDialer(u, cfg)
	if err != nil {
		return spdyDialer, nil
	}
	return portforward.NewFallbackDialer(wsDialer, spdyDialer, shouldFallback), nil
}

func shouldFallback(err error) bool {
	if err == nil {
		return false
	}
	if httpstream.IsUpgradeFailure(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "websocket") ||
		strings.Contains(msg, "bad handshake") ||
		strings.Contains(msg, "upgrade request required") ||
		strings.Contains(msg, "unknown scheme")
}

func bindAddrs(raw string) []string {
	if raw == "" {
		return []string{defaultAddress}
	}

	// Accept comma/semicolon/whitespace lists to support programmatic use and
	// future directive expansion without changing this layer.
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})
	if len(parts) == 0 {
		return []string{defaultAddress}
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	if len(out) == 0 {
		return []string{defaultAddress}
	}
	return out
}

func dialHost(addrs []string) string {
	if len(addrs) == 0 {
		return defaultAddress
	}
	host := strings.TrimSpace(addrs[0])
	if strings.EqualFold(host, "localhost") {
		return defaultAddress
	}
	if host == "" {
		return defaultAddress
	}
	return host
}

func formatPortSpec(local, remote int) string {
	if local > 0 {
		return fmt.Sprintf("%d:%d", local, remote)
	}
	return fmt.Sprintf("0:%d", remote)
}

func normalizeNetwork(raw string) (string, error) {
	network := strings.TrimSpace(raw)
	if network == "" {
		network = "tcp"
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
		return network, nil
	default:
		return "", fmt.Errorf("k8s: unsupported network for port-forward %q", network)
	}
}

func cacheKey(cfg Cfg, load loadCfg) string {
	ns := cfg.Namespace

	parts := []string{
		cfg.Label,
		cfg.Name,
		ns,
		cfg.targetRef(),
		cfg.portRef(),
		cfg.Context,
		cfg.Kubeconfig,
		cfg.Container,
		cfg.Address,
		strconv.Itoa(cfg.LocalPort),
		connprofile.BoolKey(cfg.Persist),
		cfg.PodWait.String(),
		strconv.Itoa(cfg.Retries),
		string(load.policy),
		connprofile.BoolKey(load.stdinUnavail),
		load.stdinMsg,
		strings.Join(load.allowlist, ","),
	}
	return strings.Join(parts, "|")
}

func loadOptFromCfg(cfg loadCfg) LoadOpt {
	return LoadOpt{
		ExecPolicy:             cfg.policy,
		ExecAllowlist:          append([]string(nil), cfg.allowlist...),
		StdinUnavailable:       cfg.stdinUnavail,
		StdinUnavailableSet:    true,
		StdinUnavailableReason: cfg.stdinMsg,
	}
}
