package exec

import "testing"

type flowStub struct {
	order        *[]string
	pending      *RequestResult
	cond         *RequestResult
	pre          *RequestResult
	prepare      *RequestResult
	preview      RequestResult
	useGRPC      bool
	interactive  bool
	grpc         RequestResult
	ws           RequestResult
	http         RequestResult
	finishedCall *int
}

func (f flowStub) mark(name string) {
	if f.order == nil {
		return
	}
	*f.order = append(*f.order, name)
}

func (f flowStub) PendingCancel() *RequestResult {
	f.mark("pending")
	return f.pending
}

func (f flowStub) Finish() {
	f.mark("finish")
	if f.finishedCall != nil {
		*f.finishedCall++
	}
}

func (f flowStub) EvaluateCondition() *RequestResult {
	f.mark("condition")
	return f.cond
}

func (f flowStub) RunPreRequest() *RequestResult {
	f.mark("pre")
	return f.pre
}

func (f flowStub) PrepareRequest() *RequestResult {
	f.mark("prepare")
	return f.prepare
}

func (f flowStub) PreviewResult() RequestResult {
	f.mark("preview")
	return f.preview
}

func (f flowStub) UseGRPC() bool {
	f.mark("use-grpc")
	return f.useGRPC
}

func (f flowStub) IsInteractiveWebSocket() bool {
	f.mark("interactive")
	return f.interactive
}

func (f flowStub) ExecuteInteractiveWebSocket() RequestResult {
	f.mark("ws")
	return f.ws
}

func (f flowStub) ExecuteGRPC() RequestResult {
	f.mark("grpc")
	return f.grpc
}

func (f flowStub) ExecuteHTTP() RequestResult {
	f.mark("http")
	return f.http
}

func TestRunRequestPreviewShortCircuitsExecution(t *testing.T) {
	var order []string
	finished := 0
	got := RunRequest(flowStub{
		order:        &order,
		finishedCall: &finished,
		preview:      RequestResult{Preview: true, SkipReason: "preview"},
		http:         RequestResult{SkipReason: "http"},
	})

	if !got.Preview || got.SkipReason != "preview" {
		t.Fatalf("unexpected preview result: %+v", got)
	}
	if finished != 1 {
		t.Fatalf("expected finish to run once, got %d", finished)
	}
	want := []string{"pending", "condition", "pre", "prepare", "preview", "finish"}
	if len(order) != len(want) {
		t.Fatalf("unexpected call order: got %v want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("unexpected call order: got %v want %v", order, want)
		}
	}
}

func TestRunRequestSelectsGRPCBeforeHTTP(t *testing.T) {
	var order []string
	got := RunRequest(flowStub{
		order:   &order,
		useGRPC: true,
		grpc:    RequestResult{SkipReason: "grpc"},
		http:    RequestResult{SkipReason: "http"},
	})

	if got.SkipReason != "grpc" {
		t.Fatalf("expected grpc result, got %+v", got)
	}
	want := []string{"pending", "condition", "pre", "prepare", "preview", "use-grpc", "grpc", "finish"}
	if len(order) != len(want) {
		t.Fatalf("unexpected call order: got %v want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("unexpected call order: got %v want %v", order, want)
		}
	}
}

func TestRunRequestRoutesInteractiveWebSocketBeforeHTTP(t *testing.T) {
	var order []string
	got := RunRequest(flowStub{
		order:       &order,
		interactive: true,
		ws:          RequestResult{SkipReason: "ws"},
		http:        RequestResult{SkipReason: "http"},
	})

	if got.SkipReason != "ws" {
		t.Fatalf("expected websocket result, got %+v", got)
	}
	want := []string{
		"pending",
		"condition",
		"pre",
		"prepare",
		"preview",
		"use-grpc",
		"interactive",
		"ws",
		"finish",
	}
	if len(order) != len(want) {
		t.Fatalf("unexpected call order: got %v want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("unexpected call order: got %v want %v", order, want)
		}
	}
}

func TestRunRequestPendingCancelSkipsFinish(t *testing.T) {
	var order []string
	finished := 0
	got := RunRequest(flowStub{
		order:        &order,
		finishedCall: &finished,
		pending:      &RequestResult{SkipReason: "canceled"},
	})

	if got.SkipReason != "canceled" {
		t.Fatalf("unexpected pending result: %+v", got)
	}
	if finished != 0 {
		t.Fatalf("finish should not run on pending cancel, got %d", finished)
	}
	want := []string{"pending"}
	if len(order) != len(want) || order[0] != "pending" {
		t.Fatalf("unexpected call order: got %v want %v", order, want)
	}
}
