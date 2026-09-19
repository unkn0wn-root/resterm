package parser

import (
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestParseProfileSpec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args string
		want restfile.ProfileSpec
	}{
		{name: "default", args: "", want: restfile.ProfileSpec{Count: 10}},
		{name: "positional", args: "5", want: restfile.ProfileSpec{Count: 5}},
		{name: "count", args: "count=3", want: restfile.ProfileSpec{Count: 3}},
		{name: "warmup only", args: "warmup=2", want: restfile.ProfileSpec{Count: 10, Warmup: 2}},
		{name: "zero warmup", args: "warmup=0", want: restfile.ProfileSpec{Count: 10}},
		{name: "zero delay", args: "delay=0s", want: restfile.ProfileSpec{Count: 10}},
		{
			name: "all options",
			args: "count=5 warmup=2 delay=250ms",
			want: restfile.ProfileSpec{Count: 5, Warmup: 2, Delay: 250 * time.Millisecond},
		},
		{
			name: "upper case keys",
			args: "COUNT=4 Delay=1s",
			want: restfile.ProfileSpec{Count: 4, Delay: time.Second},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			doc := Parse("profile.http", []byte("# @profile "+test.args+"\nGET https://example.com\n"))
			if len(doc.Errors) != 0 {
				t.Fatalf("Parse() errors = %+v", doc.Errors)
			}
			got := doc.Requests[0].Metadata.Profile
			if got == nil {
				t.Fatal("Parse() profile = nil")
			}
			test.want.Line = 1
			if *got != test.want {
				t.Fatalf("Parse() profile = %+v, want %+v", *got, test.want)
			}
		})
	}
}

func TestParseProfileSpecErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "zero positional", src: "# @profile 0", want: "count must be a positive integer"},
		{name: "negative positional", src: "# @profile -3", want: "count must be a positive integer"},
		{name: "word positional", src: "# @profile many", want: "count must be a positive integer"},
		{name: "positional with options", src: "# @profile 5 warmup=2", want: "use count=5"},
		{name: "zero count", src: "# @profile count=0", want: "count must be a positive integer"},
		{name: "text count", src: "# @profile count=ten", want: "count must be a positive integer"},
		{name: "empty count", src: "# @profile count=", want: `count must be a positive integer, got ""`},
		{name: "bare count", src: "# @profile count", want: "count must be a positive integer"},
		{name: "negative warmup", src: "# @profile warmup=-1", want: "warmup must be a non-negative integer"},
		{name: "empty warmup", src: "# @profile warmup=", want: `warmup must be a non-negative integer, got ""`},
		{name: "bad delay", src: "# @profile delay=soon", want: "delay must be a non-negative duration"},
		{name: "negative delay", src: "# @profile delay=-1s", want: "delay must be a non-negative duration"},
		{name: "empty delay", src: "# @profile delay=", want: `delay must be a non-negative duration, got ""`},
		{name: "unknown", src: "# @profile cnt=5", want: `unknown @profile option "cnt"`},
		{name: "repeated", src: "# @profile count=1 count=2", want: "is repeated"},
		{name: "grpc", src: "# @profile count=2\nGRPC example.Service/Call\n{}", want: "not supported for gRPC"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			src := test.src
			if !strings.Contains(src, "\n") {
				src += "\nGET https://example.com"
			}
			doc := Parse("profile.http", []byte(src+"\n"))
			var msgs []string
			for _, e := range doc.Errors {
				msgs = append(msgs, e.Message)
			}
			if got := strings.Join(msgs, "\n"); !strings.Contains(got, test.want) {
				t.Fatalf("Parse() errors = %q, want substring %q", got, test.want)
			}
			if test.name != "grpc" && doc.Requests[0].Metadata.Profile != nil {
				t.Fatalf("Parse() kept profile %+v", *doc.Requests[0].Metadata.Profile)
			}
		})
	}
}
