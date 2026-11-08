package coding

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"testing/iotest"
)

func gzipped(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	if _, err := z.Write([]byte(s)); err != nil || z.Close() != nil {
		t.Fatalf("gzip: %v", err)
	}
	return b.Bytes()
}

func TestDecode(t *testing.T) {
	gz := gzipped(t, "hello")
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
		{name: "none", enc: "none", data: gz, limit: 5},
		{name: "gzip", enc: "gzip", data: gz, limit: 5, want: []byte("hello")},
		{name: "gzip in capitals", enc: "GZIP", data: gz, limit: 5, want: []byte("hello")},
		{name: "deflate", enc: "deflate", data: gz, limit: 5, err: ErrUnsupported},
		{name: "two codings", enc: "gzip, br", data: gz, limit: 5, err: ErrUnsupported},
		{name: "not gzip", enc: "gzip", data: []byte("hello"), limit: 5, err: ErrHeader},
		{name: "truncated", enc: "gzip", data: gz[:len(gz)-4], limit: 5, err: io.ErrUnexpectedEOF},
		{name: "over the limit", enc: "gzip", data: gz, limit: 4, err: ErrTooLarge},
		{name: "no limit", enc: "gzip", data: gz, want: []byte("hello")},
		{name: "largest limit", enc: "gzip", data: gz, limit: math.MaxInt64, want: []byte("hello")},
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

type countReader struct {
	r     io.Reader
	reads int
}

func (c *countReader) Read(p []byte) (int, error) {
	c.reads++
	return c.r.Read(p)
}

func TestNewReader(t *testing.T) {
	gz := gzipped(t, "hello")
	reset := errors.New("connection reset")
	tests := []struct {
		name  string
		enc   string
		src   io.Reader
		plain bool
		want  string
		err   error
	}{
		{name: "no coding", enc: "", plain: true},
		{name: "identity", enc: "identity", plain: true},
		{name: "none", enc: " None ", plain: true},
		{name: "br", enc: "br", plain: true, err: ErrUnsupported},
		{name: "gzip", enc: "gzip", src: bytes.NewReader(gz), want: "hello"},
		{name: "empty body", enc: "gzip", src: strings.NewReader("")},
		{name: "not gzip", enc: "gzip", src: strings.NewReader("hello"), err: ErrHeader},
		{name: "short header", enc: "gzip", src: bytes.NewReader(gz[:4]), err: ErrHeader},
		{name: "source error", enc: "gzip", src: iotest.ErrReader(reset), err: reset},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &countReader{r: tt.src}
			r, err := NewReader(src, tt.enc)
			if src.reads != 0 {
				t.Fatalf("NewReader read the body %d times, want none", src.reads)
			}
			if (r == nil) != tt.plain {
				t.Fatalf("NewReader() reader = %v, want nil %t", r, tt.plain)
			}
			var got []byte
			if r != nil {
				got, err = io.ReadAll(r)
			}
			if !errors.Is(err, tt.err) || errors.Is(err, ErrHeader) != errors.Is(tt.err, ErrHeader) {
				t.Fatalf("error = %v, want %v", err, tt.err)
			}
			if string(got) != tt.want {
				t.Fatalf("read %q, want %q", got, tt.want)
			}
		})
	}
}
