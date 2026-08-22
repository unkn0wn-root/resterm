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
