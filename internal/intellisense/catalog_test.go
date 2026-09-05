package intellisense

import (
	"strings"
	"testing"
)

func TestArgumentCatalogHasUnambiguousNames(t *testing.T) {
	for name, args := range argTable {
		t.Run(name.String(), func(t *testing.T) {
			type spelling struct {
				key  string
				form argForm
			}
			seen := make(map[spelling]bool)
			for _, arg := range args.named {
				names := append([]string{arg.key}, arg.aliases...)
				for _, key := range names {
					id := spelling{strings.ToLower(key), arg.form}
					if key == "" || seen[id] {
						t.Errorf("ambiguous argument %q with form %d", key, arg.form)
					}
					seen[id] = true
				}
			}
		})
	}
}
