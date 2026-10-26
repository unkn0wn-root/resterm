package intellisense

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
	"github.com/unkn0wn-root/resterm/internal/parser"
)

func TestArgumentCatalogHasUnambiguousNames(t *testing.T) {
	for name, args := range argTable {
		t.Run(name.String(), func(t *testing.T) {
			type spelling struct {
				key string
				op  directive.Op
			}
			seen := make(map[spelling]bool)
			for _, arg := range args.named {
				names := append([]string{arg.key}, arg.aliases...)
				for _, key := range names {
					id := spelling{strings.ToLower(key), arg.op}
					if key == "" || seen[id] {
						t.Errorf("ambiguous argument %q with operator %q", key, arg.op)
					}
					seen[id] = true
				}
			}
		})
	}
}

func TestStreamArgsMatchTheParser(t *testing.T) {
	tests := []struct {
		name directive.Name
		src  string
		keys []string
	}{
		{directive.SSE, "### r\n# @sse %s\nGET http://x\n", []string{
			"duration", "idle", "idle-timeout", "limit-bytes", "max-bytes", "max-event-bytes", "max-events",
			"max-line-bytes", "timeout",
		}},
		{directive.WebSocket, "### r\n# @websocket %s\nWS wss://x\n", []string{
			"compression", "idle", "idle-timeout", "max-message-bytes", "subprotocol", "subprotocols", "timeout",
		}},
	}
	for _, tt := range tests {
		var keys []string
		for _, arg := range argsFor(tt.name).named {
			if arg.op != directive.OpEq {
				continue
			}
			for _, key := range append([]string{arg.key}, arg.aliases...) {
				keys = append(keys, key)
				t.Run(tt.name.Tag()+" "+key, func(t *testing.T) {
					doc := parser.Parse("t.http", []byte(fmt.Sprintf(tt.src, key)))
					var want []string
					if !slices.Equal(arg.value.choices, []string{"true", "false"}) {
						want = []string{
							fmt.Sprintf("%s option %q needs a value. Write it as %s=value", tt.name.Tag(), key, key),
						}
					}
					var got []string
					for _, e := range doc.Errors {
						got = append(got, e.Message)
					}
					if !slices.Equal(got, want) {
						t.Fatalf("errors = %q, want %q", got, want)
					}
					if len(doc.Warnings) > 0 {
						t.Fatalf("warnings = %v, want none", doc.Warnings)
					}
				})
			}
		}
		slices.Sort(keys)
		if !slices.Equal(keys, tt.keys) {
			t.Errorf("%s options = %q, want the parser's %q", tt.name.Tag(), keys, tt.keys)
		}
	}
}
