package ui

import (
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

func TestHeaderStatusFromHTTP(t *testing.T) {
	websocketHeaders := make(http.Header)
	websocketHeaders.Set(httpx.StreamHeaderType, " WebSocket ")

	tests := []struct {
		name  string
		resp  *httpx.Response
		label string
		level statusLevel
	}{
		{name: "nil"},
		{
			name:  "client error",
			resp:  &httpx.Response{StatusCode: http.StatusTooManyRequests},
			label: "429",
			level: statusWarn,
		},
		{
			name:  "ordinary upgrade",
			resp:  &httpx.Response{StatusCode: http.StatusSwitchingProtocols},
			label: "101",
			level: statusError,
		},
		{
			name: "websocket upgrade",
			resp: &httpx.Response{
				StatusCode: http.StatusSwitchingProtocols,
				Headers:    websocketHeaders,
			},
			label: "101",
			level: statusSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerStatusFromHTTP(tt.resp)
			if got.label != tt.label || got.level != tt.level {
				t.Fatalf("headerStatusFromHTTP() = %+v, want label %q level %d", got, tt.label, tt.level)
			}
		})
	}
}

func TestHeaderStatusFromGRPC(t *testing.T) {
	tests := []struct {
		name  string
		resp  *grpcx.Response
		label string
		level statusLevel
	}{
		{name: "nil"},
		{name: "ok", resp: &grpcx.Response{StatusCode: codes.OK}, label: "OK", level: statusSuccess},
		{
			name:  "error",
			resp:  &grpcx.Response{StatusCode: codes.NotFound, StatusMessage: "server message"},
			label: "NotFound",
			level: statusWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := headerStatusFromGRPC(tt.resp)
			if got.label != tt.label || got.level != tt.level {
				t.Fatalf("headerStatusFromGRPC() = %+v, want label %q level %d", got, tt.label, tt.level)
			}
		})
	}
}

func TestRecordHeaderTelemetry(t *testing.T) {
	m := &Model{latencySeries: newLatencySeries(latCap)}
	m.recordHeaderTelemetry(responseMsg{
		response: &httpx.Response{StatusCode: http.StatusCreated, Duration: 120 * time.Millisecond},
		latGen:   m.latencySeries.generation(),
	})

	if got := m.headerTransport.label; got != "201" {
		t.Fatalf("header status = %q, want 201", got)
	}
	if _, ok := m.latencySeries.summary(); !ok {
		t.Fatal("expected response latency to be recorded")
	}

	m.recordHeaderTelemetry(responseMsg{latGen: m.latencySeries.generation()})
	if got := m.headerTransport.label; got != "201" {
		t.Fatalf("response without transport changed header status to %q", got)
	}
}
