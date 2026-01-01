package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
	"github.com/unkn0wn-root/resterm/internal/tlsconfig"
	"golang.org/x/net/http2"
)

func (c *Client) buildHTTPClient(opts Options) (*http.Client, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
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

	if sshPlan := opts.SSH; sshPlan != nil && sshPlan.Active() {
		cfgCopy := *sshPlan.Config
		dialer := func(ctx context.Context, network, address string) (net.Conn, error) {
			return sshPlan.Manager.DialContext(ctx, cfgCopy, network, address)
		}
		transport.Proxy = nil
		transport.DialContext = dialer
		if err := http2.ConfigureTransport(transport); err != nil {
			return nil, errdef.Wrap(errdef.CodeHTTP, err, "enable http2 over ssh")
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
