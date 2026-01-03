package httpclient

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
	"github.com/unkn0wn-root/resterm/internal/ssh"
)

type traceExtras struct {
	proto  string
	proxy  string
	tunnel bool
	ssh    string
	tls    *nettrace.TLSDetails
}

func (s *traceSession) withConn(fn func(*nettrace.ConnDetails)) {
	if s == nil || fn == nil {
		return
	}
	s.mu.Lock()
	if s.conn == nil {
		s.conn = &nettrace.ConnDetails{}
	}
	fn(s.conn)
	s.mu.Unlock()
}

func (s *traceSession) withTLS(state tls.ConnectionState) {
	details := tlsDetailsFromState(state)
	if details == nil {
		return
	}
	s.mu.Lock()
	s.tls = details
	s.mu.Unlock()
}

func (s *traceSession) detailsSnapshot() *nettrace.TraceDetails {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil && s.tls == nil {
		return nil
	}
	details := &nettrace.TraceDetails{}
	if s.conn != nil {
		details.Connection = s.conn.Clone()
	}
	if s.tls != nil {
		details.TLS = s.tls.Clone()
	}
	return details
}

func applyTraceExtras(details *nettrace.TraceDetails, extra traceExtras) *nettrace.TraceDetails {
	if details == nil && extra == (traceExtras{}) {
		return nil
	}
	if details == nil {
		details = &nettrace.TraceDetails{}
	}
	if details.Connection == nil && (extra.proto != "" || extra.proxy != "" || extra.ssh != "") {
		details.Connection = &nettrace.ConnDetails{}
	}
	if extra.proto != "" {
		details.Connection.Protocol = extra.proto
	}
	if extra.proxy != "" {
		details.Connection.Proxy = extra.proxy
		details.Connection.ProxyTunnel = extra.tunnel
	}
	if extra.ssh != "" {
		details.Connection.SSH = extra.ssh
	}
	if extra.tls != nil && details.TLS == nil {
		details.TLS = extra.tls
	}
	return details
}

func buildTraceExtras(
	req *http.Request,
	resp *http.Response,
	opts Options,
	proxy string,
) traceExtras {
	extra := traceExtras{}
	if resp != nil {
		extra.proto = resp.Proto
		if resp.TLS != nil {
			extra.tls = tlsDetailsFromState(*resp.TLS)
		}
	}
	if proxy == "" {
		proxy = proxyForRequest(req, opts, nil)
	}
	if proxy != "" {
		extra.proxy = proxy
		extra.tunnel = isProxyTunnel(req)
	}
	if opts.SSH != nil && opts.SSH.Active() {
		extra.ssh = formatSSH(opts.SSH)
	}
	return extra
}

func proxyForRequest(req *http.Request, opts Options, client *http.Client) string {
	if opts.SSH != nil && opts.SSH.Active() {
		return ""
	}
	if opts.ProxyURL != "" {
		if parsed, err := url.Parse(opts.ProxyURL); err == nil {
			return sanitizeProxyURL(parsed)
		}
		return opts.ProxyURL
	}
	if req == nil || client == nil {
		return ""
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		return ""
	}
	proxyURL, err := tr.Proxy(req)
	if err != nil || proxyURL == nil {
		return ""
	}
	return sanitizeProxyURL(proxyURL)
}

func sanitizeProxyURL(proxyURL *url.URL) string {
	if proxyURL == nil {
		return ""
	}
	clean := &url.URL{
		Scheme: proxyURL.Scheme,
		Host:   proxyURL.Host,
	}
	if clean.Host == "" {
		return proxyURL.String()
	}
	return clean.String()
}

func isProxyTunnel(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return strings.EqualFold(req.URL.Scheme, "https")
}

func formatSSH(plan *ssh.Plan) string {
	if plan == nil || plan.Config == nil {
		return ""
	}
	cfg := plan.Config
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		return ""
	}
	user := strings.TrimSpace(cfg.User)
	addr := host
	if user != "" {
		addr = user + "@" + addr
	}
	if cfg.Port > 0 {
		addr = fmt.Sprintf("%s:%d", addr, cfg.Port)
	}
	return addr
}

func mergeIPs(current []string, addrs []net.IPAddr) []string {
	for _, addr := range addrs {
		if addr.IP == nil {
			continue
		}
		current = appendUnique(current, addr.String())
	}
	return current
}

func tlsDetailsFromState(state tls.ConnectionState) *nettrace.TLSDetails {
	if state.Version == 0 && state.CipherSuite == 0 &&
		state.NegotiatedProtocol == "" && state.ServerName == "" &&
		len(state.PeerCertificates) == 0 {
		return nil
	}
	details := &nettrace.TLSDetails{
		ALPN:       state.NegotiatedProtocol,
		ServerName: state.ServerName,
		Resumed:    state.DidResume,
		Verified:   len(state.VerifiedChains) > 0,
	}
	if state.Version != 0 {
		details.Version = tls.VersionName(state.Version)
	}
	if state.CipherSuite != 0 {
		details.Cipher = tls.CipherSuiteName(state.CipherSuite)
	}
	if len(state.PeerCertificates) > 0 {
		certs := make([]nettrace.TLSCert, 0, len(state.PeerCertificates))
		for _, cert := range state.PeerCertificates {
			if cert == nil {
				continue
			}
			certs = append(certs, certDetails(cert))
		}
		if len(certs) > 0 {
			details.Certificates = certs
		}
	}
	return details
}

func certDetails(cert *x509.Certificate) nettrace.TLSCert {
	return nettrace.TLSCert{
		Subject:   certName(cert.Subject),
		Issuer:    certName(cert.Issuer),
		SANs:      certSANs(cert),
		NotBefore: cert.NotBefore,
		NotAfter:  cert.NotAfter,
		Serial:    cert.SerialNumber.String(),
	}
}

func certName(name pkix.Name) string {
	cn := strings.TrimSpace(name.CommonName)
	if cn != "" {
		return cn
	}
	return strings.TrimSpace(name.String())
}

func certSANs(cert *x509.Certificate) []string {
	if cert == nil {
		return nil
	}
	var out []string
	for _, dns := range cert.DNSNames {
		out = appendUnique(out, dns)
	}
	for _, ip := range cert.IPAddresses {
		out = appendUnique(out, ip.String())
	}
	for _, email := range cert.EmailAddresses {
		out = appendUnique(out, email)
	}
	for _, uri := range cert.URIs {
		if uri == nil {
			continue
		}
		out = appendUnique(out, uri.String())
	}
	if len(out) <= 1 {
		return out
	}
	sort.Strings(out)
	return out
}

func appendUnique(dst []string, val string) []string {
	if val == "" {
		return dst
	}
	for _, existing := range dst {
		if existing == val {
			return dst
		}
	}
	return append(dst, val)
}
