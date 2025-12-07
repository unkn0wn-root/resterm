package ui

import "strings"

type metadataHintOption struct {
	Label      string
	Aliases    []string
	Summary    string
	Insert     string
	CursorBack int
}

var metadataHintCatalog = []metadataHintOption{
	{Label: "@name", Summary: "Assign a display name to the request"},
	{Label: "@description", Aliases: []string{"@desc"}, Summary: "Add a multi-line description"},
	{Label: "@tag", Aliases: []string{"@tags"}, Summary: "Categorize the request with tags"},
	{Label: "@no-log", Aliases: []string{"@nolog"}, Summary: "Disable logging of response bodies"},
	{Label: "@log-sensitive-headers", Aliases: []string{"@log-secret-headers"}, Summary: "Permit logging sensitive headers"},
	{Label: "@auth", Summary: "Configure authentication (basic, bearer, etc.)"},
	{Label: "@setting", Summary: "Set options (transport/TLS/etc.)"},
	{Label: "@settings", Summary: "Set multiple options on one line"},
	{Label: "@timeout", Summary: "Override the request timeout"},
	{Label: "@body", Summary: "Control body processing (e.g. template expansion)"},
	{Label: "@var", Summary: "Declare a request-scoped variable"},
	{Label: "@global", Summary: "Define or override a global variable"},
	{Label: "@global-secret", Summary: "Define a secret global variable"},
	{Label: "@const", Summary: "Define a reusable constant"},
	{Label: "@script", Summary: "Start a pre-request or test script block"},
	{Label: "@capture", Summary: "Capture data from the response"},
	{Label: "@trace", Summary: "Enable HTTP tracing and latency budgets"},
	{Label: "@profile", Summary: "Run the request repeatedly with profiling"},
	{Label: "@compare", Summary: "Run the request across multiple environments"},
	{Label: "@ssh", Summary: "Send request via SSH jump host"},
	{Label: "@workflow", Summary: "Begin a workflow definition"},
	{Label: "@step", Summary: "Add a workflow step"},
	{Label: "@graphql", Summary: "Enable GraphQL request handling"},
	{Label: "@graphql-operation", Aliases: []string{"@operation"}, Summary: "Set the GraphQL operation name"},
	{Label: "@variables", Aliases: []string{"@graphql-variables"}, Summary: "Provide GraphQL variables (JSON)"},
	{Label: "@query", Aliases: []string{"@graphql-query"}, Summary: "Inline a GraphQL query"},
	{Label: "@grpc", Summary: "Configure the gRPC service target"},
	{Label: "@grpc-descriptor", Summary: "Load a gRPC descriptor set"},
	{Label: "@grpc-reflection", Summary: "Toggle gRPC reflection"},
	{Label: "@grpc-plaintext", Summary: "Force plaintext gRPC transport"},
	{Label: "@grpc-authority", Summary: "Set gRPC authority override"},
	{Label: "@grpc-metadata", Summary: "Attach gRPC metadata headers"},
	{Label: "@sse", Summary: "Enable Server-Sent Events streaming"},
	{Label: "@websocket", Summary: "Enable WebSocket streaming"},
	{Label: "@ws", Summary: "Add a WebSocket scripted step (send/ping/wait/close)"},
}

var metadataSubcommandCatalog = map[string][]metadataHintOption{
	"body": {
		{Label: "expand", Summary: "Expand templates before sending the body"},
		{Label: "expand-templates", Summary: "Synonym for expand (explicit form)"},
	},
	"profile": {
		{Label: "count=", Summary: "Number of measured runs", Insert: "count=10", CursorBack: len("10")},
		{Label: "warmup=", Summary: "Warmup runs (excluded from stats)", Insert: "warmup=2", CursorBack: len("2")},
		{Label: "delay=", Summary: "Delay between runs (e.g. 250ms)", Insert: "delay=250ms", CursorBack: len("250ms")},
	},
	"script": {
		{Label: "pre-request", Summary: "Run script before the request"},
		{Label: "test", Summary: "Run script after the response"},
	},
	"trace": {
		{Label: "enabled=true", Summary: "Turn tracing on"},
		{Label: "enabled=false", Summary: "Turn tracing off"},
		{Label: "total<=", Summary: "Set overall latency budget", Insert: "total<=400ms", CursorBack: len("400ms")},
		{Label: "total=", Summary: "Set overall latency budget (alternate syntax)", Insert: "total=400ms", CursorBack: len("400ms")},
		{Label: "dns<=", Summary: "Budget for DNS lookup", Insert: "dns<=50ms", CursorBack: len("50ms")},
		{Label: "connect<=", Summary: "Budget for TCP connect", Insert: "connect<=120ms", CursorBack: len("120ms")},
		{Label: "tls<=", Summary: "Budget for TLS handshake", Insert: "tls<=150ms", CursorBack: len("150ms")},
		{Label: "request-headers<=", Summary: "Budget for sending request headers", Insert: "request-headers<=20ms", CursorBack: len("20ms")},
		{Label: "request-body<=", Summary: "Budget for sending request body", Insert: "request-body<=100ms", CursorBack: len("100ms")},
		{Label: "ttfb<=", Summary: "Budget until first response byte", Insert: "ttfb<=200ms", CursorBack: len("200ms")},
		{Label: "transfer<=", Summary: "Budget for response transfer", Insert: "transfer<=250ms", CursorBack: len("250ms")},
		{Label: "tolerance=", Summary: "Allow extra shared tolerance", Insert: "tolerance=25ms", CursorBack: len("25ms")},
		{Label: "allowance=", Summary: "Alias for tolerance", Insert: "allowance=25ms", CursorBack: len("25ms")},
	},
	"ws": {
		{Label: "send", Summary: "Send a text frame"},
		{Label: "send-json", Summary: "Send a JSON frame"},
		{Label: "send-base64", Summary: "Send base64-decoded binary data"},
		{Label: "send-file", Summary: "Send file contents"},
		{Label: "ping", Summary: "Send a ping frame"},
		{Label: "pong", Summary: "Send a pong frame"},
		{Label: "wait", Summary: "Wait for a duration or incoming message"},
		{Label: "close", Summary: "Close the connection with code and reason"},
	},
	"compare": {
		{Label: "base=", Summary: "Set the baseline environment", Insert: "base=dev", CursorBack: len("dev")},
		{Label: "baseline=", Summary: "Alias for base", Insert: "baseline=prod", CursorBack: len("prod")},
	},
	"ssh": {
		{Label: "host=", Summary: "Jump host (supports env:VAR and templates)", Insert: "host=env:SSH_HOST", CursorBack: len("env:SSH_HOST")},
		{Label: "port=", Summary: "Port (default 22)", Insert: "port=22", CursorBack: len("22")},
		{Label: "user=", Summary: "SSH user", Insert: "user=ops", CursorBack: len("ops")},
		{Label: "password=", Summary: "Password auth", Insert: "password=env:SSH_PW", CursorBack: len("env:SSH_PW")},
		{Label: "key=", Summary: "Private key path", Insert: "key=~/.ssh/id_ed25519", CursorBack: len("~/.ssh/id_ed25519")},
		{Label: "passphrase=", Summary: "Key passphrase", Insert: "passphrase=env:SSH_KEY_PW", CursorBack: len("env:SSH_KEY_PW")},
		{Label: "agent=", Summary: "Use SSH agent (default true)", Insert: "agent=false", CursorBack: len("false")},
		{Label: "known_hosts=", Summary: "Known hosts file", Insert: "known_hosts=~/.ssh/known_hosts", CursorBack: len("~/.ssh/known_hosts")},
		{Label: "strict_hostkey=", Summary: "Toggle host key checking", Insert: "strict_hostkey=false", CursorBack: len("false")},
		{Label: "persist", Summary: "Keep tunnel open (global/file scope only)"},
		{Label: "timeout=", Summary: "SSH dial timeout", Insert: "timeout=15s", CursorBack: len("15s")},
		{Label: "keepalive=", Summary: "Server keepalive interval", Insert: "keepalive=30s", CursorBack: len("30s")},
		{Label: "retries=", Summary: "Retry count for tunnel attach", Insert: "retries=2", CursorBack: len("2")},
		{Label: "use=", Summary: "Reference named profile", Insert: "use=edge", CursorBack: len("edge")},
		{Label: "persist=", Summary: "Explicit persist toggle", Insert: "persist=true", CursorBack: len("true")},
	},
}

func filterMetadataHintOptions(base string, query string) []metadataHintOption {
	key := normalizeDirectiveKey(base)
	if key == "" {
		return filterHintOptions(metadataHintCatalog, query)
	}
	options, ok := metadataSubcommandCatalog[key]
	if !ok {
		return nil
	}
	return filterHintOptions(options, query)
}

func metadataOptionMatches(option metadataHintOption, query string) bool {
	if query == "" {
		return true
	}
	if prefixHas(option.Label, query) {
		return true
	}
	for _, alias := range option.Aliases {
		if prefixHas(alias, query) {
			return true
		}
	}
	return false
}

func prefixHas(label string, query string) bool {
	trimmed := strings.TrimPrefix(label, "@")
	return strings.HasPrefix(strings.ToLower(trimmed), query)
}

func filterHintOptions(options []metadataHintOption, query string) []metadataHintOption {
	if len(options) == 0 {
		return nil
	}
	if query == "" {
		return cloneMetadataHintOptions(options)
	}
	lower := strings.ToLower(query)
	var matches []metadataHintOption
	for _, option := range options {
		if metadataOptionMatches(option, lower) {
			matches = append(matches, option)
		}
	}
	return matches
}

func cloneMetadataHintOptions(options []metadataHintOption) []metadataHintOption {
	if len(options) == 0 {
		return nil
	}
	cloned := make([]metadataHintOption, len(options))
	copy(cloned, options)
	return cloned
}

func normalizeDirectiveKey(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimPrefix(trimmed, "@")
	return strings.ToLower(trimmed)
}
