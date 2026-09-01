package ui

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

// headerTransportStatus does not replace the last response used by scripts.
type headerTransportStatus struct {
	label string
	level statusLevel
}

func headerStatusFromHTTP(resp *httpx.Response) headerTransportStatus {
	if resp == nil || resp.StatusCode == 0 {
		return headerTransportStatus{}
	}

	level := statusLevelForHTTPStatus(resp.StatusCode)
	if resp.StatusCode == http.StatusSwitchingProtocols &&
		strings.EqualFold(strings.TrimSpace(resp.Headers.Get(httpx.StreamHeaderType)), "websocket") {
		level = statusSuccess
	}
	return headerTransportStatus{
		label: strconv.Itoa(resp.StatusCode),
		level: level,
	}
}

func headerStatusFromGRPC(resp *grpcx.Response) headerTransportStatus {
	if resp == nil {
		return headerTransportStatus{}
	}

	level := statusWarn
	if resp.OK() {
		level = statusSuccess
	}
	return headerTransportStatus{
		label: resp.StatusCode.String(),
		level: level,
	}
}

func (m *Model) recordHeaderTelemetry(msg responseMsg) {
	if msg.latGen != m.latencySeries.generation() {
		return
	}
	if msg.response != nil {
		m.headerTransport = headerStatusFromHTTP(msg.response)
		m.latencySeries.add(msg.response.Duration)
		return
	}
	if msg.grpc != nil {
		m.headerTransport = headerStatusFromGRPC(msg.grpc)
		m.latencySeries.add(msg.grpc.Duration)
	}
}

func (m *Model) resetHeaderTelemetry() {
	m.headerTransport = headerTransportStatus{}
	m.latencySeries.reset()
}
