package httpclient

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
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
	transport := &http.Transport{
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
	if opts.HTTPVersion == httpver.V10 || opts.HTTPVersion == httpver.V11 {
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}

	if opts.ProxyURL != "" {
		proxyURL, err := url.Parse(opts.ProxyURL)
		if err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "parse proxy url")
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	if opts.InsecureSkipVerify || len(opts.RootCAs) > 0 || opts.ClientCert != "" ||
		opts.ClientKey != "" {
		tlsCfg, err := tlsconfig.Build(tlsconfig.Files{
			RootCAs:    opts.RootCAs,
			RootMode:   opts.RootMode,
			ClientCert: opts.ClientCert,
			ClientKey:  opts.ClientKey,
			Insecure:   opts.InsecureSkipVerify,
		}, opts.BaseDir)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsCfg
	}

	sshOn := opts.SSH != nil && opts.SSH.Active()
	k8sOn := opts.K8s != nil && opts.K8s.Active()
	if tunnel.HasConflict(sshOn, k8sOn) {
		return nil, errdef.New(errdef.CodeHTTP, "ssh and k8s transports cannot be combined")
	}
	if strings.TrimSpace(opts.ProxyURL) != "" && (sshOn || k8sOn) {
		return nil, errdef.New(
			errdef.CodeHTTP,
			"proxy cannot be combined with ssh or k8s tunneling",
		)
	}

	applyTunnel := func(kind string, dial tunnel.DialContextFunc) error {
		if err := tunnel.ApplyHTTPTransport(transport, opts.HTTPVersion, dial); err != nil {
			return errdef.Wrap(errdef.CodeHTTP, err, "enable http2 over %s", kind)
		}
		return nil
	}

	if sshOn {
		sshPlan := opts.SSH
		cfgCopy := *sshPlan.Config
		dial := tunnel.DialerFor(sshPlan.Manager, cfgCopy)
		if err := applyTunnel("ssh", dial); err != nil {
			return nil, err
		}
	}

	if k8sOn {
		k8sPlan := opts.K8s
		cfgCopy := *k8sPlan.Config
		dial := tunnel.DialerFor(k8sPlan.Manager, cfgCopy)
		if err := applyTunnel("k8s", dial); err != nil {
			return nil, err
		}
	}

	client := &http.Client{Transport: transport, Jar: c.jar}
	if opts.Timeout > 0 {
		client.Timeout = opts.Timeout
	}
	if !opts.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client, nil
}
