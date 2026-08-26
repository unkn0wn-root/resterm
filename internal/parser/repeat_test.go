package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParsePollAndRetryPolicies(t *testing.T) {
	t.Parallel()
	doc := Parse("jobs.http", []byte(`### Wait for job
# @retry-when response.statusCode in [429, 502, 503]
# @retry-backoff exponential(100ms, 2s) jitter=20%
# @retry count=4
# @poll every=500ms timeout=30s until=response.json().status == "completed"
GET https://example.com/jobs/123
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("Parse() errors = %+v", doc.Errors)
	}
	if len(doc.Requests) != 1 {
		t.Fatalf("Parse() requests = %d, want 1", len(doc.Requests))
	}
	req := doc.Requests[0]
	poll := req.Metadata.Poll
	if poll == nil {
		t.Fatal("@poll was not parsed")
	}
	if poll.Every != 500*time.Millisecond || poll.Timeout != 30*time.Second {
		t.Fatalf("@poll timing = %+v", poll)
	}
	if got, want := poll.Until.Expression, `response.json().status == "completed"`; got != want {
		t.Fatalf("@poll until = %q, want %q", got, want)
	}
	if poll.Until.Line != 5 || poll.Until.Col <= 1 {
		t.Fatalf("@poll position = %d:%d", poll.Until.Line, poll.Until.Col)
	}

	retry := req.Metadata.Retry
	if retry == nil {
		t.Fatal("@retry was not parsed")
	}
	if retry.Count != 4 {
		t.Fatalf("@retry count = %d, want 4", retry.Count)
	}
	if retry.When == nil || retry.When.Expression != "response.statusCode in [429, 502, 503]" {
		t.Fatalf("@retry-when = %+v", retry.When)
	}
	if retry.Backoff.Initial != 100*time.Millisecond || retry.Backoff.Max != 2*time.Second || retry.Backoff.JitterPercent != 20 {
		t.Fatalf("@retry-backoff = %+v", retry.Backoff)
	}
}

func TestParsePollAndRetryDefaults(t *testing.T) {
	t.Parallel()
	doc := Parse("jobs.http", []byte(`# @poll until=response.statusCode == 200
# @retry count=1
GET https://example.com/jobs/123
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("Parse() errors = %+v", doc.Errors)
	}
	req := doc.Requests[0]
	if req.Metadata.Poll.Every != time.Second || req.Metadata.Poll.Timeout != 30*time.Second {
		t.Fatalf("@poll defaults = %+v", req.Metadata.Poll)
	}
	backoff := req.Metadata.Retry.Backoff
	if backoff.Initial != 100*time.Millisecond || backoff.Max != 2*time.Second || backoff.JitterPercent != 20 {
		t.Fatalf("@retry defaults = %+v", backoff)
	}
}

func TestParseRepeatPolicyErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "poll missing until", src: "# @poll every=1s\nGET https://example.com\n", want: "requires until="},
		{name: "zero interval", src: "# @poll every=0 timeout=1s until=true\nGET https://example.com\n", want: "positive duration"},
		{name: "retry missing count", src: "# @retry enabled=true\nGET https://example.com\n", want: "count is required"},
		{name: "retry zero", src: "# @retry count=0\nGET https://example.com\n", want: "greater than zero"},
		{name: "orphan condition", src: "# @retry-when true\nGET https://example.com\n", want: "requires @retry"},
		{name: "orphan backoff", src: "# @retry-backoff exponential(1ms, 1s)\nGET https://example.com\n", want: "requires @retry"},
		{name: "bad backoff bound", src: "# @retry count=1\n# @retry-backoff exponential(2s, 1s)\nGET https://example.com\n", want: "maximum must be at least"},
		{name: "bad jitter", src: "# @retry count=1\n# @retry-backoff exponential(1ms, 1s) jitter=101%\nGET https://example.com\n", want: "between 0% and 100%"},
		{name: "non-finite jitter", src: "# @retry count=1\n# @retry-backoff exponential(1ms, 1s) jitter=NaN%\nGET https://example.com\n", want: "between 0% and 100%"},
		{name: "profile conflict", src: "# @profile count=2\n# @retry count=1\nGET https://example.com\n", want: "cannot be combined with @profile"},
		{name: "grpc unsupported", src: "# @poll until=true\nGRPC example.Service/Call\n{}\n", want: "not supported for gRPC"},
		{name: "sse unsupported", src: "# @sse\n# @retry count=1\nGET https://example.com/events\n", want: "not supported for SSE"},
		{name: "websocket unsupported", src: "# @websocket\n# @poll until=true\nGET wss://example.com/events\n", want: "not supported for WebSocket"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc := Parse("repeat.http", []byte(test.src))
			var messages []string
			for _, parseErr := range doc.Errors {
				messages = append(messages, parseErr.Message)
			}
			if got := strings.Join(messages, "\n"); !strings.Contains(got, test.want) {
				t.Fatalf("Parse() errors = %q, want substring %q", got, test.want)
			}
		})
	}
}

func TestParsePollContinuationKeepsUntilExpression(t *testing.T) {
	t.Parallel()
	doc := Parse("jobs.http", []byte(`# @poll every=250ms timeout=2s until=(
#   response.statusCode == 200 &&
#   response.json().ready
# )
GET https://example.com/jobs/123
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("Parse() errors = %+v", doc.Errors)
	}
	expr := doc.Requests[0].Metadata.Poll.Until.Expression
	if !strings.Contains(expr, "response.json().ready") || !strings.HasSuffix(expr, ")") {
		t.Fatalf("@poll continuation expression = %q", expr)
	}
}

func TestParseRetryBackoffContinuation(t *testing.T) {
	t.Parallel()
	doc := Parse("jobs.http", []byte(`# @retry count=2
# @retry-backoff exponential(
#   100ms,
#   2s
# ) jitter=10%
GET https://example.com/jobs/123
`))
	if len(doc.Errors) != 0 {
		t.Fatalf("Parse() errors = %+v", doc.Errors)
	}
	backoff := doc.Requests[0].Metadata.Retry.Backoff
	if backoff.Initial != 100*time.Millisecond || backoff.Max != 2*time.Second || backoff.JitterPercent != 10 {
		t.Fatalf("@retry-backoff continuation = %+v", backoff)
	}
}
