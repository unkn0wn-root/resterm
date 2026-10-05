package httpx

import (
	"io"
	"strings"
	"testing"
)

func TestIdleReaderResetsOnlyWhenDataArrives(t *testing.T) {
	reset := make(chan struct{}, 1)
	r := idleReader{r: strings.NewReader("\n"), reset: reset}
	buf := make([]byte, 8)

	if n, err := r.Read(buf); n != 1 || err != nil {
		t.Fatalf("Read = %d, %v, want the blank line", n, err)
	}
	select {
	case <-reset:
	default:
		t.Fatal("a read with data did not reset the idle timer")
	}

	if n, err := r.Read(buf); n != 0 || err != io.EOF {
		t.Fatalf("Read = %d, %v, want EOF", n, err)
	}
	select {
	case <-reset:
		t.Fatal("a read without data reset the idle timer")
	default:
	}
}
