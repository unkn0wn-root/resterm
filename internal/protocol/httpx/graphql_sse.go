package httpx

import (
	"encoding/json"
	"errors"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

// graphQLSSEEvent applies graphql-sse to an event: complete ends the stream,
// and next must carry an execution result.
func graphQLSSEEvent(evt SSEEvent) (bool, error) {
	switch evt.Event {
	case "complete":
		return true, nil
	case "next":
		var res map[string]json.RawMessage
		err := json.Unmarshal([]byte(evt.Data), &res)
		var syntax *json.SyntaxError
		switch {
		case errors.As(err, &syntax):
			return false, diag.New(diag.ClassProtocol, "server sent a next event whose data is not valid JSON")
		case err != nil || res == nil:
			return false, diag.New(diag.ClassProtocol, "server sent a next event whose data is not a JSON object")
		}
	}
	return false, nil
}
