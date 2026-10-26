package restfile

import "testing"

func TestRepeatUnsupported(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{name: "http"},
		{name: "grpc", req: Request{GRPC: &GRPCRequest{}}, want: "gRPC"},
		{name: "sse", req: Request{SSE: &SSERequest{}}, want: "SSE"},
		{name: "websocket", req: Request{WebSocket: &WebSocketRequest{}}, want: "WebSocket"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.req.RepeatUnsupported(); got != test.want {
				t.Fatalf("RepeatUnsupported() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInteractiveWebSocket(t *testing.T) {
	t.Parallel()
	steps := []WebSocketStep{{Type: WebSocketStepSendText, Value: "hi"}}
	tests := []struct {
		name string
		req  Request
		want bool
	}{
		{name: "http"},
		{name: "sse", req: Request{SSE: &SSERequest{}}},
		{name: "websocket", req: Request{WebSocket: &WebSocketRequest{}}, want: true},
		{name: "websocket with steps", req: Request{WebSocket: &WebSocketRequest{Steps: steps}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.req.InteractiveWebSocket(); got != test.want {
				t.Fatalf("InteractiveWebSocket() = %v, want %v", got, test.want)
			}
		})
	}
}
