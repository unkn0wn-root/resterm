package authcmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Setenv("GO_WANT_AUTHCMD_HELPER", "1")

	tests := []struct {
		name string
		args []string
		want string
		err  string
	}{
		{
			name: "success",
			args: []string{"stdout", "token-123"},
			want: "token-123",
		},
		{
			name: "exit error",
			args: []string{"stderr-exit", "2", "denied"},
			err:  `exited with status 2: denied`,
		},
		{
			name: "stdout limit",
			args: []string{"stdout-repeat", "x", "300000"},
			err:  "stdout exceeded",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Argv: append([]string{os.Args[0], "-test.run=TestAuthCmdHelperProcess", "--"}, tt.args...),
			}
			out, err := run(context.Background(), cfg)
			if tt.err != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("run() error = %q, want substring %q", err.Error(), tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("run() error = %v", err)
			}
			if got := string(out.stdout); got != tt.want {
				t.Fatalf("stdout = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthCmdHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_AUTHCMD_HELPER") != "1" {
		return
	}

	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) == 0 {
		os.Exit(0)
	}

	switch args[0] {
	case "stdout":
		fmt.Fprint(os.Stdout, args[1])
		os.Exit(0)
	case "stderr-exit":
		fmt.Fprint(os.Stderr, args[2])
		os.Exit(2)
	case "stdout-repeat":
		count := 0
		fmt.Sscanf(args[2], "%d", &count)
		fmt.Fprint(os.Stdout, strings.Repeat(args[1], count))
		os.Exit(0)
	default:
		os.Exit(3)
	}
}
