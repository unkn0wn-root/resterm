package intellisense

import "strings"

var methods = []Item{
	{Label: "GET", Summary: "Retrieve a resource"},
	{Label: "POST", Summary: "Create or submit data"},
	{Label: "PUT", Summary: "Replace a resource"},
	{Label: "PATCH", Summary: "Partially update a resource"},
	{Label: "DELETE", Summary: "Remove a resource"},
	{Label: "HEAD", Summary: "Headers only, no body"},
	{Label: "OPTIONS", Summary: "Query supported methods"},
	{Label: "TRACE", Summary: "Echo the received request"},
	{Label: "CONNECT", Summary: "Establish a tunnel"},
	{Label: "WS", Summary: "Open a WebSocket connection"},
	{Label: "WSS", Summary: "Open a secure WebSocket connection"},
	{Label: "GRPC", Summary: "Call a gRPC method"},
}

var methodKeywords = func() map[string]struct{} {
	out := make(map[string]struct{}, len(methods))
	for _, it := range methods {
		out[it.Label] = struct{}{}
	}
	return out
}()

func IsMethodKeyword(token string) bool {
	_, ok := methodKeywords[strings.ToUpper(token)]
	return ok
}

type methodSource struct{}

func (methodSource) Provide(ctx Context, _ Scope) []Item {
	if ctx.Kind != KindMethod {
		return nil
	}
	return filter(methods, ctx.Query)
}
