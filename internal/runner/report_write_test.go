package runner

import (
	"errors"
	"testing"
)

func TestReportWriteNilWriter(t *testing.T) {
	rep := &Report{}

	if err := rep.WriteText(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteText(nil): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteJSON(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteJSON(nil): got %v want %v", err, ErrNilWriter)
	}
	if err := rep.WriteJUnit(nil); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("WriteJUnit(nil): got %v want %v", err, ErrNilWriter)
	}
}
