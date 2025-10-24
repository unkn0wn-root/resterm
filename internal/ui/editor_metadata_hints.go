package ui

import "strings"

type metadataHintOption struct {
	Label   string
	Aliases []string
	Summary string
}

var metadataHintCatalog = []metadataHintOption{
	{Label: "@name", Summary: "Assign a display name to the request"},
	{Label: "@description", Aliases: []string{"@desc"}, Summary: "Add a multi-line description"},
	{Label: "@tag", Aliases: []string{"@tags"}, Summary: "Categorize the request with tags"},
	{Label: "@no-log", Aliases: []string{"@nolog"}, Summary: "Disable logging of response bodies"},
	{Label: "@log-sensitive-headers", Aliases: []string{"@log-secret-headers"}, Summary: "Permit logging sensitive headers"},
	{Label: "@auth", Summary: "Configure authentication (basic, bearer, etc.)"},
	{Label: "@setting", Summary: "Set per-request options (e.g. retries, proxies)"},
	{Label: "@timeout", Summary: "Override the request timeout"},
	{Label: "@body", Summary: "Control body processing (e.g. template expansion)"},
	{Label: "@var", Summary: "Declare a request-scoped variable"},
	{Label: "@global", Summary: "Define or override a global variable"},
	{Label: "@global-secret", Summary: "Define a secret global variable"},
	{Label: "@const", Summary: "Define a reusable constant"},
	{Label: "@script", Summary: "Start a pre-request or test script block"},
	{Label: "@capture", Summary: "Capture data from the response"},
	{Label: "@profile", Summary: "Run the request repeatedly with profiling"},
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
	"script": {
		{Label: "pre-request", Summary: "Run script before the request"},
		{Label: "test", Summary: "Run script after the response"},
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
