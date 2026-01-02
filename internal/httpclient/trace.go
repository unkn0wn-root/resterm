package httpclient

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/unkn0wn-root/resterm/internal/nettrace"
)

type traceSession struct {
	collector      *nettrace.Collector
	trace          *httptrace.ClientTrace
	reqBodyActive  bool
	ttfbActive     bool
	transferActive bool
	mu             sync.Mutex
	conn           *nettrace.ConnDetails
	tls            *nettrace.TLSDetails
}

func newTraceSession() *traceSession {
	s := &traceSession{collector: nettrace.NewCollector()}
	s.trace = &httptrace.ClientTrace{
		GetConn:              s.onGetConn,
		DNSStart:             s.onDNSStart,
		DNSDone:              s.onDNSDone,
		ConnectStart:         s.onConnectStart,
		ConnectDone:          s.onConnectDone,
		GotConn:              s.onGotConn,
		TLSHandshakeStart:    s.onTLSHandshakeStart,
		TLSHandshakeDone:     s.onTLSHandshakeDone,
		WroteHeaders:         s.onWroteHeaders,
		WroteRequest:         s.onWroteRequest,
		GotFirstResponseByte: s.onGotFirstResponseByte,
	}
	return s
}

func (s *traceSession) bind(req *http.Request) *http.Request {
	if req == nil {
		return nil
	}

	ctx := httptrace.WithClientTrace(req.Context(), s.trace)
	return req.WithContext(ctx)
}

func (s *traceSession) onGetConn(hostPort string) {
	if hostPort == "" {
		return
	}
	s.withConn(func(conn *nettrace.ConnDetails) {
		if conn.DialAddr == "" {
			conn.DialAddr = hostPort
		}
	})
}

func (s *traceSession) onDNSStart(info httptrace.DNSStartInfo) {
	now := time.Now()
	s.collector.Begin(nettrace.PhaseDNS, now)
	host := info.Host
	if host != "" {
		s.collector.UpdateMeta(nettrace.PhaseDNS, func(meta *nettrace.PhaseMeta) {
			meta.Addr = host
		})
	}
}

func (s *traceSession) onDNSDone(info httptrace.DNSDoneInfo) {
	now := time.Now()
	s.collector.UpdateMeta(nettrace.PhaseDNS, func(meta *nettrace.PhaseMeta) {
		if len(info.Addrs) > 0 {
			meta.Addr = info.Addrs[0].String()
		}

		if info.Coalesced {
			meta.Cached = true
		}
	})
	if len(info.Addrs) > 0 {
		s.withConn(func(conn *nettrace.ConnDetails) {
			conn.ResolvedAddrs = mergeIPs(conn.ResolvedAddrs, info.Addrs)
		})
	}
	s.collector.End(nettrace.PhaseDNS, now, info.Err)
	if info.Err != nil {
		s.collector.Fail(info.Err)
	}
}

func (s *traceSession) onConnectStart(network, addr string) {
	now := time.Now()
	s.collector.Begin(nettrace.PhaseConnect, now)
	if addr != "" {
		s.collector.UpdateMeta(nettrace.PhaseConnect, func(meta *nettrace.PhaseMeta) {
			meta.Addr = addr
		})
	}
	if network != "" || addr != "" {
		s.withConn(func(conn *nettrace.ConnDetails) {
			if network != "" {
				conn.Network = network
			}
			if addr != "" {
				conn.DialAddr = addr
			}
		})
	}
}

func (s *traceSession) onConnectDone(network, addr string, err error) {
	s.collector.End(nettrace.PhaseConnect, time.Now(), err)
	if err != nil {
		s.collector.Fail(err)
	}
}

func (s *traceSession) onGotConn(info httptrace.GotConnInfo) {
	s.withConn(func(conn *nettrace.ConnDetails) {
		conn.Reused = info.Reused
		conn.WasIdle = info.WasIdle
		conn.IdleTime = info.IdleTime
		if info.Conn != nil {
			conn.LocalAddr = localAddrString(info.Conn)
			conn.RemoteAddr = addrString(info.Conn)
		}
	})
	if !info.Reused {
		return
	}

	now := time.Now()
	s.collector.Begin(nettrace.PhaseConnect, now)
	s.collector.UpdateMeta(nettrace.PhaseConnect, func(meta *nettrace.PhaseMeta) {
		meta.Reused = true
		if info.Conn != nil {
			meta.Addr = addrString(info.Conn)
		}
	})
	s.collector.End(nettrace.PhaseConnect, now, nil)
}

func (s *traceSession) onTLSHandshakeStart() {
	s.collector.Begin(nettrace.PhaseTLS, time.Now())
}

func (s *traceSession) onTLSHandshakeDone(state tls.ConnectionState, err error) {
	s.collector.End(nettrace.PhaseTLS, time.Now(), err)
	if err != nil {
		s.collector.Fail(err)
	}
	s.withTLS(state)
}

func (s *traceSession) onWroteHeaders() {
	now := time.Now()
	s.collector.Begin(nettrace.PhaseReqHdrs, now)
	s.collector.End(nettrace.PhaseReqHdrs, now, nil)
	if !s.reqBodyActive {
		s.reqBodyActive = true
		s.collector.Begin(nettrace.PhaseReqBody, now)
	}
}

func (s *traceSession) onWroteRequest(info httptrace.WroteRequestInfo) {
	now := time.Now()
	if s.reqBodyActive {
		s.collector.End(nettrace.PhaseReqBody, now, info.Err)
		s.reqBodyActive = false
	}
	if info.Err != nil {
		s.collector.Fail(info.Err)
		return
	}
	if !s.ttfbActive {
		s.ttfbActive = true
		s.collector.Begin(nettrace.PhaseTTFB, now)
	}
}

func (s *traceSession) onGotFirstResponseByte() {
	now := time.Now()
	if s.ttfbActive {
		s.collector.End(nettrace.PhaseTTFB, now, nil)
		s.ttfbActive = false
	}
	if !s.transferActive {
		s.transferActive = true
		s.collector.Begin(nettrace.PhaseTransfer, now)
	}
}

func (s *traceSession) finishTransfer(err error) {
	if !s.transferActive {
		return
	}

	s.collector.End(nettrace.PhaseTransfer, time.Now(), err)
	s.transferActive = false
	if err != nil {
		s.collector.Fail(err)
	}
}

func (s *traceSession) fail(err error) {
	s.collector.Fail(err)
}

func (s *traceSession) complete(extra traceExtras) *nettrace.Timeline {
	s.collector.Complete(time.Now())
	tl := s.collector.Timeline()
	if tl == nil {
		return nil
	}
	details := s.detailsSnapshot()
	if details = applyTraceExtras(details, extra); details != nil {
		tl.Details = details
	}
	return tl
}

func addrString(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	return conn.RemoteAddr().String()
}

func localAddrString(conn net.Conn) string {
	if conn == nil {
		return ""
	}
	return conn.LocalAddr().String()
}
