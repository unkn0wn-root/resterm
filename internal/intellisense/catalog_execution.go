package intellisense

import (
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/delay"
	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func addExecutionArgs(c argumentCatalog) {
	c.add(args{named: mockArgs()}, directive.Mock)
	c.add(args{named: matchArgs}, directive.Match)
	c.add(args{named: []argument{opt("calls", "Exact matching request count", "1")}}, directive.Expect)
	c.add(args{named: profileArgs}, directive.Profile)
	c.add(args{named: pollArgs}, directive.Poll)
	c.add(args{named: []argument{opt("count", "Number of additional attempts", "4")}}, directive.Retry)
	c.add(args{named: []argument{
		word("exponential", "Starting and maximum retry delay").call("exponential(100ms, 2s)"),
		opt("jitter", "Random delay percentage", "20%"),
	}}, directive.RetryBackoff)
	c.add(args{named: scriptArgs}, directive.Script)
	c.add(args{named: []argument{word("pre-request", "Run RestermScript before the request")}}, directive.RTS)
	c.add(args{
		value: directiveValue(filePath("module", "RestermScript module", PathRTS, pathWord)),
		named: []argument{word("as", "Bind the module to an alias")},
	}, directive.Use)
	c.add(args{named: traceArgs}, directive.Trace)

	failure := choice("on-failure", "Failure behavior",
		string(restfile.WorkflowOnFailureStop), string(restfile.WorkflowOnFailureContinue),
	).alias("onfailure")
	c.add(args{named: []argument{failure}}, directive.Workflow)
	c.add(args{named: []argument{
		failure, optValue("using", "Request to run", namesValue(requestNames)).alias("run"),
	}}, directive.Step)
	c.add(args{named: []argument{
		optValue("run", "Request to run", namesValue(requestNames)).alias("using"),
		opt("fail", "Fail workflow branch with message", `"message"`),
	}}, directive.If, directive.Elif, directive.Else, directive.Case, directive.Default)
}

func requestNames(_ Context, sc Scope) ([]string, string) {
	return sc.RequestNames, "request"
}

var matchArgs = []argument{
	opt("query", "Query matcher rules as JSON", `{"key":"value"}`),
	opt("headers", "Header matcher rules as JSON", `{"X-Key":"value"}`),
	opt("json", "Literal JSON body subset", `{"key":"value"}`),
	opt("json-rules", "JSON body matcher rules", `{"key":{"gt":1}}`),
}

func mockArgs() []argument {
	latency := opt("latency", "Constant response latency", "250ms")
	for _, d := range delay.Distributions() {
		latency.examples = append(latency.examples, argExample{
			text: d.Usage(), placeholder: d.Args, summary: d.Summary, call: d.Name,
		})
	}
	return []argument{
		choice("method", "HTTP method to match", httpMethods...),
		opt("path", "Origin-form route path", "/resource"),
		opt("name", "Scenario selector name", "success"),
		opt("sequence", "Response sequence name", "polling"),
		opt("sequence-key", "Per-key cursor source (path, query, header, or cookie)", "path.id"),
		flag("default", "Use as the route fallback"),
		latency,
		flag("interpolate", "Expand response templates"),
	}
}

var httpMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodConnect,
	http.MethodOptions,
	http.MethodTrace,
}

var profileArgs = []argument{
	opt("count", "Number of measured runs", "10"),
	opt("warmup", "Warmup runs (excluded from stats)", "2"),
	opt("delay", "Delay between runs (e.g. 250ms)", "250ms"),
}

var pollArgs = []argument{
	opt("every", "Time between polling requests", "500ms"),
	opt("timeout", "Total time allowed for polling", "30s"),
	opt("until", "Condition that stops polling (must be last)", `response.json().status == "completed"`),
}

var scriptArgs = []argument{
	word("pre-request", "Run script before the request").chains(),
	word("test", "Run script after the response").chains(),
	choice("lang", "Script language", "rts", "js").alias("language"),
}

var traceArgs = []argument{
	flag("enabled", "Toggle tracing"),
	budget("total", "Set overall latency budget", "400ms"),
	opt("total", "Set overall latency budget (alternate syntax)", "400ms"),
	budget("dns", "Budget for DNS lookup", "50ms"),
	budget("connect", "Budget for TCP connect", "120ms"),
	budget("tls", "Budget for TLS handshake", "150ms"),
	budget("request-headers", "Budget for sending request headers", "20ms"),
	budget("request-body", "Budget for sending request body", "100ms"),
	budget("ttfb", "Budget until first response byte", "200ms"),
	budget("transfer", "Budget for response transfer", "250ms"),
	opt("tolerance", "Allow extra shared tolerance", "25ms").alias("allowance"),
}
