package httpx

// Resterm adds these headers so streaming results can use the normal response,
// history, and rendering paths. They do not come from the server.
const (
	StreamHeaderType    = "X-Resterm-Stream-Type"
	StreamHeaderSummary = "X-Resterm-Stream-Summary"

	streamContentTypeJSON = "application/json; charset=utf-8"
)
