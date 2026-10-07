package intellisense

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/directive"
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
