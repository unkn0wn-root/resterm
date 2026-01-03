package nettrace

import "time"

type TraceDetails struct {
	Connection *ConnDetails
	TLS        *TLSDetails
}

func (d *TraceDetails) Clone() *TraceDetails {
	if d == nil {
		return nil
	}
	clone := &TraceDetails{}
	if d.Connection != nil {
		clone.Connection = d.Connection.Clone()
	}
	if d.TLS != nil {
		clone.TLS = d.TLS.Clone()
	}
	return clone
}

type ConnDetails struct {
	Reused        bool
	WasIdle       bool
	IdleTime      time.Duration
	Network       string
	DialAddr      string
	LocalAddr     string
	RemoteAddr    string
	ResolvedAddrs []string
	Proxy         string
	ProxyTunnel   bool
	SSH           string
	Protocol      string
}

func (c *ConnDetails) Clone() *ConnDetails {
	if c == nil {
		return nil
	}
	clone := *c
	if len(c.ResolvedAddrs) > 0 {
		clone.ResolvedAddrs = append([]string(nil), c.ResolvedAddrs...)
	}
	return &clone
}

type TLSDetails struct {
	Version      string
	Cipher       string
	ALPN         string
	ServerName   string
	Resumed      bool
	Verified     bool
	Certificates []TLSCert
}

func (t *TLSDetails) Clone() *TLSDetails {
	if t == nil {
		return nil
	}
	clone := *t
	if len(t.Certificates) > 0 {
		clone.Certificates = make([]TLSCert, len(t.Certificates))
		for i, cert := range t.Certificates {
			clone.Certificates[i] = cert.Clone()
		}
	}
	return &clone
}

type TLSCert struct {
	Subject   string
	Issuer    string
	SANs      []string
	NotBefore time.Time
	NotAfter  time.Time
	Serial    string
}

func (c TLSCert) Clone() TLSCert {
	clone := c
	if len(c.SANs) > 0 {
		clone.SANs = append([]string(nil), c.SANs...)
	}
	return clone
}
