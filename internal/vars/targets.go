package vars

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

// Target is one compare run target. Profile is set only when a group is
// varied, so Name falls back to the environment label for flat compares.
type Target struct {
	Profile string
	Env     Environment
}

func (t Target) Name() string {
	if t.Profile != "" {
		return t.Profile
	}
	return t.Env.Label()
}

// CompareTargets resolves compare target names against the catalog and
// validates that baseline, when set, matches one of them.
func (c Catalog) CompareTargets(
	base Selection,
	group, baseline string,
	names []string,
) ([]Target, error) {
	group = str.Trim(group)
	baseline = str.Trim(baseline)
	switch {
	case c.Grouped() && group == "":
		return nil, diag.New(diag.ClassParse, "grouped compare requires a group")
	case !c.Grouped() && group != "":
		return nil, diag.New(diag.ClassParse, "compare group requires grouped environments")
	}

	if c.Grouped() {
		g, ok := c.findGroup(group)
		if !ok {
			return nil, diag.Newf(diag.ClassParse, "unknown environment group %q", group)
		}
		group = g.Name
	}

	out := make([]Target, 0, len(names))
	found := baseline == ""
	for _, name := range names {
		var (
			sel Selection
			err error
		)
		if group != "" {
			sel, err = c.Select("", base.WithGroup(group, name).Groups())
		} else {
			sel, err = c.Select(name, nil)
		}
		if err != nil {
			return nil, err
		}
		env, err := c.Resolve(sel)
		if err != nil {
			return nil, err
		}
		t := Target{Env: env}
		if group != "" {
			t.Profile, _ = env.sel.Profile(group)
		}
		if strings.EqualFold(t.Name(), baseline) {
			found = true
		}
		out = append(out, t)
	}
	if !found {
		return nil, diag.Newf(
			diag.ClassParse,
			"compare baseline %q must match a compare target",
			baseline,
		)
	}
	return out, nil
}
