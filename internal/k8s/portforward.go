package k8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"k8s.io/apimachinery/pkg/util/httpstream"
	appsv1client "k8s.io/client-go/kubernetes/typed/apps/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	spdytransport "k8s.io/client-go/transport/spdy"
	"k8s.io/client-go/util/flowcontrol"
)

type clusterClients struct {
	apps appsv1client.AppsV1Interface
	core corev1client.CoreV1Interface
}

func startSession(ctx context.Context, cfg execConfig, load loadSettings) (*session, error) {
	ensureRuntimeDiagInstalled()

	restCfg, err := restConfig(cfg.Config, loadOptionsFromSettings(load))
	if err != nil {
		return nil, err
	}

	clients, err := newClusterClients(restCfg)
	if err != nil {
		return nil, fmt.Errorf("k8s: build client: %w", err)
	}

	resolver := newClusterResolver(clients, cfg.Namespace)
	target, err := resolver.resolveForwardTarget(ctx, cfg)
	if err != nil {
		return nil, err
	}

	u := clients.core.
		RESTClient().
		Post().
		Resource("pods").
		Namespace(cfg.Namespace).
		Name(target.pod).
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
		[]string{formatPortSpec(cfg.LocalPort, target.port)},
		stopCh,
		readyCh,
		io.Discard,
		io.Discard,
	)
	if err != nil {
		return nil, fmt.Errorf("k8s: build port-forwarder: %w", err)
	}

	ses := &session{
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

	ses.localAddr = net.JoinHostPort(
		addressedDialHost(addresses),
		strconv.Itoa(int(ports[0].Local)),
	)
	ses.setDiag(newDiagCollector(int(ports[0].Local), target.port, diagCap))
	return ses, nil
}

func newClusterClients(cfg *rest.Config) (clusterClients, error) {
	if cfg == nil {
		return clusterClients{}, errors.New("missing rest config")
	}

	shallowCopy := *cfg
	if shallowCopy.UserAgent == "" {
		shallowCopy.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	httpClient, err := rest.HTTPClientFor(&shallowCopy)
	if err != nil {
		return clusterClients{}, err
	}
	if shallowCopy.RateLimiter == nil && shallowCopy.QPS > 0 {
		if shallowCopy.Burst <= 0 {
			return clusterClients{}, fmt.Errorf(
				"burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0",
			)
		}
		shallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(
			shallowCopy.QPS,
			shallowCopy.Burst,
		)
	}

	appsClient, err := appsv1client.NewForConfigAndClient(&shallowCopy, httpClient)
	if err != nil {
		return clusterClients{}, err
	}
	coreClient, err := corev1client.NewForConfigAndClient(&shallowCopy, httpClient)
	if err != nil {
		return clusterClients{}, err
	}
	return clusterClients{apps: appsClient, core: coreClient}, nil
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
	return httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err)
}

func bindAddrs(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})

	seen := make(map[string]struct{}, len(parts))
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

func addressedDialHost(addrs []string) string {
	if len(addrs) == 0 {
		return defaultAddress
	}

	host := strings.TrimSpace(addrs[0])
	switch {
	case host == "":
		return defaultAddress
	case strings.EqualFold(host, "localhost"):
		return defaultAddress
	default:
		return host
	}
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
