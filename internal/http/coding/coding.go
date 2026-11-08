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

	"github.com/unkn0wn-root/resterm/internal/bytesize"
)

var (
	// ErrUnsupported means the content coding is not identity, none or gzip.
	ErrUnsupported = errors.New("unsupported content coding")
	// ErrHeader means a gzip body does not start with a valid gzip header.
	ErrHeader = errors.New("invalid gzip header")
	// ErrTooLarge means the decoded body is over the limit.
	ErrTooLarge = errors.New("decoded body exceeds the limit")
)

// NewReader returns a reader that decodes r, or nil when enc names no coding.
func NewReader(r io.Reader, enc string) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(enc)) {
	case "", "identity", "none":
		return nil, nil
	case "gzip":
		return &gzipReader{src: r}, nil
	default:
		return nil, fmt.Errorf("%w %q", ErrUnsupported, enc)
	}
}

// gzipReader reads the gzip header on its first Read, so a stream can start before its first byte.
type gzipReader struct {
	src io.Reader
	zr  *gzip.Reader
	err error
}

func (r *gzipReader) Read(p []byte) (int, error) {
	if r.zr == nil && r.err == nil {
		r.zr, r.err = gzip.NewReader(r.src)
		// io.EOF stays as it is, so an empty body reads as empty.
		if errors.Is(r.err, gzip.ErrHeader) || errors.Is(r.err, io.ErrUnexpectedEOF) {
			r.err = fmt.Errorf("%w: %w", ErrHeader, r.err)
		}
	}
	if r.err != nil {
		return 0, r.err
	}
	return r.zr.Read(p)
}

// Decode undoes the content coding enc, matched without regard to case. It
// returns nil and no error when data is empty, or when enc is empty, identity
// or none, so the caller keeps data as it is. Only gzip is decoded. A decoded
// body over limit bytes returns ErrTooLarge, and a limit of 0 or less means no
// limit. Gzip data that breaks off returns the compress/gzip error.
func Decode(data []byte, enc string, limit int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	r, err := NewReader(bytes.NewReader(data), enc)
	if err != nil || r == nil {
		return nil, err
	}
	if limit > 0 {
		r = io.LimitReader(r, bytesize.Add(limit, 1))
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if limit > 0 && int64(len(out)) > limit {
		return nil, ErrTooLarge
	}
	return out, nil
}
