package intellisense

var headerNames = []Item{
	{Label: "Accept", Summary: "Acceptable response media types"},
	{Label: "Accept-Encoding", Summary: "Acceptable content encodings"},
	{Label: "Accept-Language", Summary: "Preferred languages"},
	{Label: "Authorization", Summary: "Authentication credentials"},
	{Label: "Cache-Control", Summary: "Caching directives"},
	{Label: "Connection", Summary: "Connection control"},
	{Label: "Content-Type", Summary: "Request body media type"},
	{Label: "Content-Length", Summary: "Body size in bytes"},
	{Label: "Content-Encoding", Summary: "Body encoding"},
	{Label: "Cookie", Summary: "Stored cookies"},
	{Label: "Host", Summary: "Target host and port"},
	{Label: "If-Match", Summary: "Conditional on ETag match"},
	{Label: "If-None-Match", Summary: "Conditional on ETag mismatch"},
	{Label: "If-Modified-Since", Summary: "Conditional on modification time"},
	{Label: "Origin", Summary: "Request origin"},
	{Label: "Range", Summary: "Request a byte range"},
	{Label: "Referer", Summary: "Referring page"},
	{Label: "User-Agent", Summary: "Client identifier"},
	{Label: "X-Api-Key", Summary: "API key header"},
	{Label: "X-Request-Id", Summary: "Request correlation id"},
	{Label: "X-Forwarded-For", Summary: "Originating client address"},
}

var headerValues = map[string][]Item{
	"content-type": {
		{Label: "application/json"},
		{Label: "application/xml"},
		{Label: "application/x-www-form-urlencoded"},
		{Label: "multipart/form-data"},
		{Label: "text/plain"},
		{Label: "application/octet-stream"},
		{Label: "application/graphql"},
		{Label: "text/event-stream"},
	},
	"accept": {
		{Label: "application/json"},
		{Label: "application/xml"},
		{Label: "text/plain"},
		{Label: "text/event-stream"},
		{Label: "*/*"},
	},
	"accept-encoding": {
		{Label: "gzip"},
		{Label: "deflate"},
		{Label: "br"},
		{Label: "identity"},
	},
	"cache-control": {
		{Label: "no-cache"},
		{Label: "no-store"},
		{Label: "max-age=0"},
	},
	"connection": {
		{Label: "keep-alive"},
		{Label: "close"},
	},
}

type headerSource struct{}

func (headerSource) Provide(ctx Context, _ Scope) []Item {
	switch ctx.Kind {
	case KindHeaderName:
		return filter(headerNameItems, ctx.Query)
	case KindHeaderValue:
		return filter(headerValues[ctx.Directive], ctx.Query)
	default:
		return nil
	}
}

var headerNameItems = func() []Item {
	out := make([]Item, len(headerNames))
	for i, h := range headerNames {
		h.Insert = h.Label + ":"
		out[i] = h
	}
	return out
}()
