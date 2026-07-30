package cli

import (
	"fmt"
	"slices"
	"strings"

	str "github.com/unkn0wn-root/resterm/internal/util"
)

type GroupFlags map[string]string

func (f *GroupFlags) Set(value string) error {
	group, profile, ok := strings.Cut(value, "=")
	group = str.Trim(group)
	profile = str.Trim(profile)
	if !ok || group == "" || profile == "" {
		return fmt.Errorf("expected group=profile")
	}
	if *f == nil {
		*f = make(GroupFlags)
	}
	for name := range *f {
		if strings.EqualFold(name, group) {
			return fmt.Errorf("group %q selected more than once", group)
		}
	}
	(*f)[group] = profile
	return nil
}

func (f GroupFlags) String() string {
	names := make([]string, 0, len(f))
	for name := range f {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	out := make([]string, len(names))
	for i, name := range names {
		out[i] = name + "=" + f[name]
	}
	return strings.Join(out, ",")
}
