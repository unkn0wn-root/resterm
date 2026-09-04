package intellisense

import (
	"maps"
	"slices"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/directive"
)

// compareTarget is the internal key for positional environment names.
const compareTarget = "target"

func compareArgs() args {
	return args{
		value: directiveValue(optValue(compareTarget, "Environment to compare", namesValue(compareTargets)).repeats()),
		named: []argument{
			optValue(
				"base",
				"Set the baseline environment",
				namesValue(compareBaseline),
			).alias("baseline", "primary", "ref"),
			optValue("group", "Select the environment group to vary", namesValue(compareGroups)),
		},
		extraItems: compareGroupItems,
	}
}

func compareGroupItems(ctx Context, sc Scope) []Item {
	if ctx.completed.has("group") {
		return nil
	}
	var items []Item
	for _, group := range groupNames(sc.EnvironmentGroups) {
		items = append(items, Item{
			Label:    "group=" + group,
			Insert:   "group=" + directive.Quote(group),
			Summary:  "environment group",
			Continue: true,
		})
	}
	return items
}

func compareGroups(_ Context, sc Scope) ([]string, string) {
	return groupNames(sc.EnvironmentGroups), "environment group"
}

func compareBaseline(ctx Context, sc Scope) ([]string, string) {
	if targets := ctx.completed[compareTarget]; len(targets) > 0 {
		return targets, "environment profile"
	}
	return compareTargets(ctx, sc)
}

func compareTargets(ctx Context, sc Scope) ([]string, string) {
	if len(sc.Environments) > 0 {
		return sc.Environments, "environment"
	}
	return groupedProfiles(ctx, sc), "environment profile"
}

func groupedProfiles(ctx Context, sc Scope) []string {
	if selected, ok := ctx.completed.last("group"); ok {
		for group, profiles := range sc.EnvironmentGroups {
			if strings.EqualFold(group, selected) {
				return slices.Clone(profiles)
			}
		}
	}
	return groupProfiles(sc.EnvironmentGroups)
}

func groupNames(groups map[string][]string) []string {
	return slices.SortedFunc(maps.Keys(groups), compareFold)
}

func groupProfiles(groups map[string][]string) []string {
	seen := make(map[string]struct{})
	var profiles []string
	for _, group := range groupNames(groups) {
		for _, profile := range groups[group] {
			key := strings.ToLower(profile)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			profiles = append(profiles, profile)
		}
	}
	slices.SortFunc(profiles, compareFold)
	return profiles
}

func compareFold(a, b string) int {
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}
