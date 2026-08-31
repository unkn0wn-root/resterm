package httpx

import (
	"cmp"
	"fmt"
	"io"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/diag"
)

const DefaultMaxResponseBytes = 32 << 20

type ResponseTooLargeError struct {
	Limit int64
}

func (e *ResponseTooLargeError) Error() string {
	return fmt.Sprintf("response body exceeds the %d byte limit", e.Limit)
}

func (o Options) responseLimit() int64 {
	return o.MaxResponseBytes.Or(DefaultMaxResponseBytes)
}

func (o Options) readBody(r io.Reader) ([]byte, error) {
	body, err := readResponseBody(r, o.responseLimit())
	if err != nil {
		return nil, diag.WrapAs(
			diag.ClassProtocol,
			err,
			"read response body",
			diag.WithComponent(diag.ComponentHTTP),
		)
	}
	return body, nil
}

func readResponseBody(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return io.ReadAll(r)
	}

	body, err := io.ReadAll(io.LimitReader(r, bytesize.Add(limit, 1)))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, &ResponseTooLargeError{Limit: limit}
	}
	return body, nil
}

const defaultWebSocketMessageBytes = 32 << 10

func webSocketReadLimit(configured, setting int64) int64 {
	return cmp.Or(configured, setting, defaultWebSocketMessageBytes)
}
