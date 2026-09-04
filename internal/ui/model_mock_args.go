package ui

import (
	"errors"
	"fmt"

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

const (
	mockSourceFlagName  = "source"
	mockSourceFlagAlias = "s"
)

func (a mockStartArgs) scoped() bool {
	return a.all || a.recursive || len(a.sources) > 0
}

// parseMockStartArgs reads the grammar
//
//	[host:port] [--addr host:port] [--source file[,file]]... [--recursive] [--all]
func parseMockStartArgs(args []string) (mockStartArgs, error) {
	var out mockStartArgs
	fs := newMockStartFlagSet(&out)

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, cli.ErrHelp) {
			return out, errMockArgsUsage
		}
		return out, err
	}
	for _, arg := range fs.Args() {
		if err := out.setAddr(arg); err != nil {
			return out, err
		}
	}
	if out.all && len(out.sources) > 0 {
		return out, errors.New("--all cannot be combined with --source")
	}
	return out, nil
}

func newMockStartFlagSet(out *mockStartArgs) *cli.FlagSet {
	fs := cli.NewFlagSet("mock start")
	fs.StringVarAliases(&out.addr, "", "Listen address", "addr", "a")
	fs.StringListVarAliases(&out.sources, "Serve only these request files", mockSourceFlagName, mockSourceFlagAlias)
	fs.BoolVarAliases(&out.recursive, false, "Scan the workspace recursively", "recursive", "r")
	fs.BoolVarAliases(&out.all, false, "Serve the whole workspace", "all")
	return fs
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
