package update

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

type Progress interface {
	Start(total int64)
	Advance(n int64)
	Finish()
}

type progressWriter struct {
	progress Progress
}

func (w progressWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.progress.Advance(int64(len(p)))
	}
	return len(p), nil
}

func (c Client) download(ctx context.Context, a Asset, dst string, prog Progress) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	if a.URL == "" {
		return sum, fmt.Errorf("empty asset url")
	}

	res, err := c.get(ctx, a.URL, "asset")
	if err != nil {
		return sum, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return sum, fmt.Errorf("open temp file: %w", err)
	}

	var reader io.Reader = res.Body
	if a.Size > 0 {
		// one byte past the expected size so an oversized body fails the exact-size check early
		reader = io.LimitReader(reader, a.Size+1)
	}
	if prog != nil {
		total := a.Size
		if total <= 0 && res.ContentLength > 0 {
			total = res.ContentLength
		}
		prog.Start(total)
		defer prog.Finish()
		reader = io.TeeReader(reader, progressWriter{progress: prog})
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), reader)
	cerr := f.Close()
	if err != nil {
		return sum, fmt.Errorf("write asset: %w", err)
	}
	if cerr != nil {
		return sum, fmt.Errorf("close temp file: %w", cerr)
	}

	if a.Size > 0 && n != a.Size {
		return sum, fmt.Errorf("download size mismatch: got %d want %d", n, a.Size)
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
