package coding

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"testing"
)

func TestDecode(t *testing.T) {
	var zipped bytes.Buffer
	z := gzip.NewWriter(&zipped)
	if _, err := z.Write([]byte("hello")); err != nil || z.Close() != nil {
		t.Fatalf("gzip: %v", err)
	}
	gz := zipped.Bytes()
	tests := []struct {
		name  string
		enc   string
		data  []byte
		limit int64
		want  []byte
		err   error
	}{
		{name: "no coding", enc: "", data: gz, limit: 5},
		{name: "identity", enc: " Identity ", data: gz, limit: 5},
		{name: "gzip", enc: "gzip", data: gz, limit: 5, want: []byte("hello")},
		{name: "gzip in capitals", enc: "GZIP", data: gz, limit: 5, want: []byte("hello")},
		{name: "deflate", enc: "deflate", data: gz, limit: 5, err: ErrUnsupported},
		{name: "two codings", enc: "gzip, br", data: gz, limit: 5, err: ErrUnsupported},
		{name: "not gzip", enc: "gzip", data: []byte("hello"), limit: 5, err: ErrHeader},
		{name: "truncated", enc: "gzip", data: gz[:len(gz)-4], limit: 5, err: io.ErrUnexpectedEOF},
		{name: "over the limit", enc: "gzip", data: gz, limit: 4, err: ErrTooLarge},
		{name: "empty gzip", enc: "gzip", data: []byte{}, limit: 5},
		{name: "empty br", enc: "br", data: []byte{}, limit: 5},
		{name: "nil gzip", enc: "gzip", limit: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.data, tt.enc, tt.limit)
			if !errors.Is(err, tt.err) || (err == nil) != (tt.err == nil) {
				t.Fatalf("Decode() error = %v, want %v", err, tt.err)
			}
			if (got == nil) != (tt.want == nil) || !bytes.Equal(got, tt.want) {
				t.Fatalf("Decode() = %q, want %q", got, tt.want)
			}
		})
	}
}
