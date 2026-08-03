package grpcclient

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
)

func TestApplyOptionSettings(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		want     Options
	}{
		{
			name:     "sizes and compression",
			settings: map[string]string{"grpc-max-recv-size": "8MB", "grpc-compression": "gzip"},
			want:     Options{MaxRecvSize: 8 << 20, Compression: "gzip"},
		},
		{
			name:     "send size",
			settings: map[string]string{"grpc-max-send-size": "1KiB"},
			want:     Options{MaxSendSize: 1024},
		},
		{
			name:     "plain byte count",
			settings: map[string]string{"grpc-max-recv-size": "4096"},
			want:     Options{MaxRecvSize: 4096},
		},
		{
			name:     "none clears compression",
			settings: map[string]string{"grpc-compression": "none"},
			want:     Options{},
		},
		{
			name:     "keys are case and space insensitive",
			settings: map[string]string{"  GRPC-MAX-RECV-SIZE ": "2MB"},
			want:     Options{MaxRecvSize: 2 << 20},
		},
		{
			name:     "unrelated keys are ignored",
			settings: map[string]string{"timeout": "30s"},
			want:     Options{},
		},
		{
			name:     "no settings",
			settings: nil,
			want:     Options{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Options
			if err := ApplyOptionSettings(&got, tt.settings); err != nil {
				t.Fatalf("apply: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("options = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestApplyOptionSettingsRejectsBadValues(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		want     string
	}{
		{
			name:     "unparseable size",
			settings: map[string]string{"grpc-max-recv-size": "huge"},
			want:     `invalid grpc-max-recv-size "huge" (use a size such as 8MB)`,
		},
		{
			name:     "empty size reads as missing",
			settings: map[string]string{"grpc-max-send-size": "  "},
			want:     "missing grpc-max-send-size value (use a size such as 8MB)",
		},
		{
			name:     "zero is out of range",
			settings: map[string]string{"grpc-max-recv-size": "0"},
			want:     "invalid grpc-max-recv-size",
		},
		{
			name:     "unknown compressor",
			settings: map[string]string{"grpc-compression": "snappy"},
			want:     `invalid grpc-compression "snappy" (use gzip or none)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts Options
			err := ApplyOptionSettings(&opts, tt.settings)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestApplyOptionSettingsLenientKeepsCurrentValue(t *testing.T) {
	opts := Options{MaxRecvSize: 1024, Compression: "gzip"}
	err := applyOptionSettings(&opts, map[string]string{
		"grpc-max-recv-size": "huge",
		"grpc-compression":   "snappy",
	}, false)
	if err != nil {
		t.Fatalf("lenient apply returned %v", err)
	}
	if opts.MaxRecvSize != 1024 || opts.Compression != "gzip" {
		t.Fatalf("options = %+v, want the originals kept", opts)
	}
}

func TestCallOptionsCarrySizesAndCompression(t *testing.T) {
	gr, opt := descriptorRequest(t)
	opt.MaxRecvSize = 8 << 20
	opt.MaxSendSize = 4 << 20
	opt.Compression = "gzip"

	conn := &stubConn{}
	if _, err := stubClient(conn).Execute(
		context.Background(),
		&restfile.Request{GRPC: gr},
		opt,
		nil,
	); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var recv, send, compress int
	for _, o := range conn.calls {
		switch o.(type) {
		case grpc.MaxRecvMsgSizeCallOption:
			recv++
		case grpc.MaxSendMsgSizeCallOption:
			send++
		case grpc.CompressorCallOption:
			compress++
		}
	}
	if recv != 1 || send != 1 || compress != 1 {
		t.Fatalf("call options recv=%d send=%d compress=%d, want 1 each", recv, send, compress)
	}
}

func TestCallOptionsAreEmptyByDefault(t *testing.T) {
	if got := callOptions(Options{}); len(got) != 0 {
		t.Fatalf("callOptions() = %v, want none", got)
	}
}
