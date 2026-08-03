package filelookup

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

type recordFS struct {
	files map[string][]byte
	err   error
	tried []string
}

func (r *recordFS) ReadFile(name string) ([]byte, error) {
	r.tried = append(r.tried, name)
	if data, ok := r.files[name]; ok {
		return data, nil
	}
	if r.err != nil {
		return nil, r.err
	}
	return nil, os.ErrNotExist
}

func TestFor(t *testing.T) {
	fb := []string{"/fb"}
	tests := []struct {
		name       string
		baseDir    string
		noFallback bool
		want       Lookup
	}{
		{
			name:       "no fallback with base dir drops raw",
			baseDir:    "/base",
			noFallback: true,
			want:       Lookup{BaseDir: "/base"},
		},
		{
			name:       "no fallback without base dir keeps raw",
			noFallback: true,
			want:       Lookup{AllowRaw: true},
		},
		{
			name:    "fallback enabled keeps roots and raw",
			baseDir: "/base",
			want:    Lookup{BaseDir: "/base", Fallbacks: fb, AllowRaw: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := For(tt.baseDir, fb, tt.noFallback)
			if got.BaseDir != tt.want.BaseDir ||
				got.AllowRaw != tt.want.AllowRaw ||
				!slices.Equal(got.Fallbacks, tt.want.Fallbacks) {
				t.Fatalf("For() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCandidateOrder(t *testing.T) {
	tests := []struct {
		name   string
		lookup Lookup
		want   []string
	}{
		{
			name:   "base dir first, then fallbacks, then raw",
			lookup: Lookup{BaseDir: "/base", Fallbacks: []string{"/a", "/b"}, AllowRaw: true},
			want:   []string{"/base/x.json", "/a/x.json", "/b/x.json", "x.json"},
		},
		{
			name:   "raw only",
			lookup: Lookup{AllowRaw: true},
			want:   []string{"x.json"},
		},
		{
			name:   "empty fallbacks are skipped",
			lookup: Lookup{BaseDir: "/base", Fallbacks: []string{"", "/a"}},
			want:   []string{"/base/x.json", "/a/x.json"},
		},
		{
			name:   "duplicates collapse",
			lookup: Lookup{BaseDir: "/base", Fallbacks: []string{"/base"}, AllowRaw: true},
			want:   []string{"/base/x.json", "x.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.lookup.Candidates("x.json")
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Candidates() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadUsesAbsolutePathAsGiven(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "abs", "x.json")
	fs := &recordFS{files: map[string][]byte{abs: []byte("ok")}}

	data, tried, err := Lookup{BaseDir: "/base"}.Read(fs, abs)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "ok" || tried != abs {
		t.Fatalf("read = %q from %q, want ok from %q", data, tried, abs)
	}
	if len(fs.tried) != 1 {
		t.Fatalf("tried = %v, want a single attempt", fs.tried)
	}
}

func TestReadStopsOnFatalError(t *testing.T) {
	fs := &recordFS{err: os.ErrPermission}

	_, tried, err := Lookup{BaseDir: "/base", Fallbacks: []string{"/a"}}.Read(fs, "x.json")
	if err == nil {
		t.Fatal("expected the permission error to surface")
	}
	if tried != "/base/x.json" || len(fs.tried) != 1 {
		t.Fatalf("tried = %v, want only the base dir candidate", fs.tried)
	}
}

func TestFatal(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"permission", os.ErrPermission, true},
		{"is a directory", syscall.EISDIR, true},
		{"wrapped is a directory", &os.PathError{Err: syscall.EISDIR}, true},
		{"invalid", os.ErrInvalid, true},
		{"not exist", os.ErrNotExist, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Fatal(tt.err); got != tt.want {
				t.Fatalf("Fatal(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
