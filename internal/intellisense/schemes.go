package intellisense

// schemes are URL schemes completed at the start of a request URL. They are
// prefix filtered so a target that does not start with one of them (e.g. a gRPC
// or CONNECT host:port) simply does not match and no popup appears.
var schemes = []Item{
	{Label: "https://", Summary: "HTTP over TLS"},
	{Label: "http://", Summary: "HTTP"},
	{Label: "wss://", Summary: "WebSocket over TLS"},
	{Label: "ws://", Summary: "WebSocket"},
}

type schemeSource struct{}

func (schemeSource) Provide(ctx Context, _ Scope) []Item {
	if ctx.Kind != KindScheme {
		return nil
	}
	return filter(schemes, ctx.Query)
}
