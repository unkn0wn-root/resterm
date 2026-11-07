// Package coding undoes HTTP content codings, the values of a Content-Encoding
// header (RFC 9110, section 8.4.1).
package coding

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	// ErrUnsupported means the content coding is not identity or gzip.
	ErrUnsupported = errors.New("unsupported content coding")
	// ErrHeader means a gzip body does not start with a valid gzip header.
	ErrHeader = errors.New("invalid gzip header")
	// ErrTooLarge means the decoded body is over the limit.
	ErrTooLarge = errors.New("decoded body exceeds the limit")
)

// Decode undoes the content coding enc, matched without regard to case. It
// returns nil and no error when data is empty, or when enc is empty or
// identity, so the caller keeps data as it is. Only gzip is decoded. A decoded
// body over limit bytes returns ErrTooLarge, and gzip data that breaks off
// returns the compress/gzip error.
func Decode(data []byte, enc string, limit int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "", "identity":
		return nil, nil
	case "gzip":
	default:
		return nil, ErrUnsupported
	}

	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrHeader, err)
	}
	out, err := io.ReadAll(io.LimitReader(r, limit+1))
	if closeErr := r.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > limit {
		return nil, ErrTooLarge
	}
	return out, nil
}
