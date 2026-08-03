package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestParseMockStartArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want mockStartArgs
	}{
		{name: "empty", args: nil, want: mockStartArgs{}},
		{name: "positional address", args: []string{"127.0.0.1:9090"}, want: mockStartArgs{addr: "127.0.0.1:9090"}},
		{name: "addr flag", args: []string{"--addr", "127.0.0.1:9090"}, want: mockStartArgs{addr: "127.0.0.1:9090"}},
		{name: "addr equals", args: []string{"--addr=127.0.0.1:9090"}, want: mockStartArgs{addr: "127.0.0.1:9090"}},
		{
			name: "comma list",
			args: []string{"--source", "users.http,payments.http"},
			want: mockStartArgs{sources: []string{"users.http,payments.http"}},
		},
		{
			name: "repeated short source",
			args: []string{"-s", "users.http", "-s", "payments.http"},
			want: mockStartArgs{sources: []string{"users.http", "payments.http"}},
		},
		{
			name: "source then address",
			args: []string{"--source", "users.http", "127.0.0.1:9090"},
			want: mockStartArgs{addr: "127.0.0.1:9090", sources: []string{"users.http"}},
		},
		{
			name: "address then source",
			args: []string{"127.0.0.1:9090", "--source", "users.http"},
			want: mockStartArgs{addr: "127.0.0.1:9090", sources: []string{"users.http"}},
		},
		{name: "recursive", args: []string{"-r"}, want: mockStartArgs{recursive: true}},
		{name: "all", args: []string{"--all"}, want: mockStartArgs{all: true}},
		{
			name: "recursive with source",
			args: []string{"--recursive", "--source", "users.http"},
			want: mockStartArgs{recursive: true, sources: []string{"users.http"}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseMockStartArgs(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if got.addr != test.want.addr || !slices.Equal(got.sources, test.want.sources) ||
				got.recursive != test.want.recursive || got.all != test.want.all {
				t.Fatalf("parseMockStartArgs(%q) = %+v, want %+v", test.args, got, test.want)
			}
		})
	}
}

func TestParseMockStartArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing source value", args: []string{"--source"}, want: "needs an argument"},
		{name: "address twice", args: []string{"127.0.0.1:1", "--addr", "127.0.0.1:2"}, want: "more than once"},
		{name: "second positional", args: []string{"127.0.0.1:1", "127.0.0.1:2"}, want: "more than once"},
		{name: "unknown flag", args: []string{"--sauce", "users.http"}, want: "not defined"},
		{name: "bare request file", args: []string{"users.http"}, want: "use --source"},
		{name: "all with source", args: []string{"--all", "--source", "users.http"}, want: "--all cannot be combined"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseMockStartArgs(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("parseMockStartArgs(%q) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestParseMockStartArgsReportsUsageRequest(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		if _, err := parseMockStartArgs([]string{arg}); !errors.Is(err, errMockArgsUsage) {
			t.Fatalf("parseMockStartArgs(%q) error = %v, want errMockArgsUsage", arg, err)
		}
	}
}

func TestResolveStartSpecScope(t *testing.T) {
	model := newMockTestModel(t, mockTestDocument)
	root := model.mockRoot()
	api := filepath.Join(root, "api.http")

	spec, err := model.resolveStartSpec(mockStartArgs{sources: []string{"api.http,./api.http", api}})
	if err != nil {
		t.Fatal(err)
	}
	if spec.src.Path != root || !slices.Equal(spec.src.Files, []string{api}) {
		t.Fatalf("spec.src = %+v, want one resolved file %s under %s", spec.src, api, root)
	}

	spec, err = model.resolveStartSpec(mockStartArgs{recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if spec.src.Path != root || !spec.src.Recursive || spec.src.Files != nil {
		t.Fatalf("workspace spec.src = %+v, want recursive workspace scope", spec.src)
	}

	// A recursive workspace must not turn a listed scope into the --recursive
	// conflict, because the user never asked to recurse.
	model.ws.recursive = true
	spec, err = model.resolveStartSpec(mockStartArgs{sources: []string{"api.http"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.src.Files) != 1 || spec.src.Recursive {
		t.Fatalf("spec.src = %+v, want one listed file and no recursion", spec.src)
	}

	spec, err = model.resolveStartSpec(mockStartArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if !spec.src.Recursive {
		t.Fatalf("spec.src = %+v, want the workspace recursion default", spec.src)
	}
}
