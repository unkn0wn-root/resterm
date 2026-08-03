package ui

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/files"
)

// errMockArgsUsage marks -h and --help, so callers render the usage line
// instead of an error.
var errMockArgsUsage = errors.New("usage requested")

// mockStartArgs holds :mock start and :mock restart arguments as typed. sources
// stay raw flag values, commas and all, until mock.NewSources resolves them.
type mockStartArgs struct {
	addr      string
	sources   []string
	recursive bool
	all       bool
}

func (a mockStartArgs) scoped() bool {
	return a.all || a.recursive || len(a.sources) > 0
}

// parseMockStartArgs reads the grammar
//
//	[host:port] [--addr host:port] [--source file[,file]]... [--recursive] [--all]
func parseMockStartArgs(args []string) (mockStartArgs, error) {
	var out mockStartArgs
	fs := cli.NewFlagSet("mock start")
	cli.StringVarAliases(fs, &out.addr, "", "Listen address", "addr", "a")
	cli.StringListVarAliases(fs, &out.sources, "Serve only these request files", "source", "s")
	cli.BoolVarAliases(fs, &out.recursive, false, "Scan the workspace recursively", "recursive", "r")
	cli.BoolVarAliases(fs, &out.all, false, "Serve the whole workspace", "all")

	// flag stops parsing at the first non-flag argument, so lift a leading
	// address out of the way first.
	var pos []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		pos = append(pos, args[0])
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return out, errMockArgsUsage
		}
		return out, err
	}
	for _, arg := range append(pos, fs.Args()...) {
		if err := out.setAddr(arg); err != nil {
			return out, err
		}
	}
	if out.all && len(out.sources) > 0 {
		return out, errors.New("--all cannot be combined with --source")
	}
	return out, nil
}

func (a *mockStartArgs) setAddr(addr string) error {
	if files.IsRequest(addr) {
		return fmt.Errorf("unexpected argument %s (use --source)", addr)
	}
	if a.addr != "" {
		return errors.New("address given more than once")
	}
	a.addr = addr
	return nil
}
