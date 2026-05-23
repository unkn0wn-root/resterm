package httpclient

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/httpver"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"github.com/unkn0wn-root/resterm/internal/tunnel"
)

const (
	defaultDialTimeout           = 30 * time.Second
	defaultDialKeepAlive         = 30 * time.Second
	defaultTLSHandshakeTimeout   = 10 * time.Second
	defaultMaxIdleConns          = 100
	defaultIdleConnTimeout       = 90 * time.Second
	defaultExpectContinueTimeout = time.Second
)

func (c *Client) buildHTTPClient(opts Options) (*http.Client, error) {
	transport := newBaseTransport()
	applyTransportHTTPVersion(transport, opts.HTTPVersion)

	if err := applyProxy(transport, opts); err != nil {
		return nil, err
	}
	if err := applyTLS(transport, opts); err != nil {
		return nil, err
	}
	if err := applyTunnels(transport, opts); err != nil {
		return nil, err
	}

	return newHTTPClient(transport, opts), nil
}

func newBaseTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultDialTimeout,
			KeepAlive: defaultDialKeepAlive,
		}).DialContext,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		MaxIdleConns:          defaultMaxIdleConns,
		IdleConnTimeout:       defaultIdleConnTimeout,
		ExpectContinueTimeout: defaultExpectContinueTimeout,
		ForceAttemptHTTP2:     true,
	}
}

func applyTransportHTTPVersion(transport *http.Transport, version httpver.Version) {
	if transport == nil {
		return
	}
	if version == httpver.V10 || version == httpver.V11 {
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
}

func applyProxy(transport *http.Transport, opts Options) error {
	if opts.ProxyURL == "" {
		return nil
	}

	proxyURL, err := url.Parse(opts.ProxyURL)
	if err != nil {
		return diag.WrapAs(
			diag.ClassProtocol,
			err,
			"parse proxy url",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}
	transport.Proxy = http.ProxyURL(proxyURL)
	return nil
}

func applyTLS(transport *http.Transport, opts Options) error {
	if !needsTLSConfig(opts) {
		return nil
	}

	tlsCfg, err := tlsconfig.Build(tlsconfig.Files{
		RootCAs:    opts.RootCAs,
		RootMode:   opts.RootMode,
		ClientCert: opts.ClientCert,
		ClientKey:  opts.ClientKey,
		Insecure:   opts.InsecureSkipVerify,
	}, opts.BaseDir)
	if err != nil {
		return err
	}
	transport.TLSClientConfig = tlsCfg
	return nil
}

func needsTLSConfig(opts Options) bool {
	return opts.InsecureSkipVerify ||
		len(opts.RootCAs) > 0 ||
		opts.ClientCert != "" ||
		opts.ClientKey != ""
}

func applyTunnels(transport *http.Transport, opts Options) error {
	sshOn := opts.SSH != nil && opts.SSH.Active()
	k8sOn := opts.K8s != nil && opts.K8s.Active()
	if tunnel.HasConflict(sshOn, k8sOn) {
		return diag.New(diag.ClassRoute, "ssh and k8s transports cannot be combined")
	}
	if opts.ProxyURL != "" && (sshOn || k8sOn) {
		return diag.New(
			diag.ClassRoute,
			"proxy cannot be combined with ssh or k8s tunneling",
		)
	}

	if sshOn {
		sshPlan := opts.SSH
		cfgCopy := *sshPlan.Config
		dial := tunnel.DialerFor(sshPlan.Manager, cfgCopy)
		if err := applyTunnel(transport, opts.HTTPVersion, "ssh", dial); err != nil {
			return err
		}
	}

	if k8sOn {
		k8sPlan := opts.K8s
		cfgCopy := *k8sPlan.Config
		dial := tunnel.DialerFor(k8sPlan.Manager, cfgCopy)
		if err := applyTunnel(transport, opts.HTTPVersion, "k8s", dial); err != nil {
			return err
		}
	}

	return nil
}

func applyTunnel(
	transport *http.Transport,
	version httpver.Version,
	kind string,
	dial tunnel.DialContextFunc,
) error {
	if err := tunnel.ApplyHTTPTransport(transport, version, dial); err != nil {
		return diag.WrapAsf(diag.ClassRoute, err, "enable http2 over %s", kind)
	}
	return nil
}

func newHTTPClient(transport *http.Transport, opts Options) *http.Client {
	client := &http.Client{Transport: transport, Jar: opts.CookieJar}
	if opts.Timeout > 0 {
		client.Timeout = opts.Timeout
	}
	if !opts.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}
