package vars

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

// Catalog holds every selectable environment from one env file. It is either
// flat (a list of named environments) or grouped (independent groups where
// picking one profile per group builds the environment). The zero value is a
// valid empty catalog.
type Catalog struct {
	envs   []entry
	groups []Group
	shared map[string]string
	source string
}

type Group struct {
	Name     string
	Default  string
	Profiles EnvironmentSet
}

type entry struct {
	name   string
	values map[string]string
}

func NewCatalog(set EnvironmentSet) (Catalog, error) {
	seen := newNames("environment", len(set))
	envs := make([]entry, 0, len(set))
	var shared map[string]string

	for raw, values := range set {
		name, err := seen.add(raw)
		if err != nil {
			return Catalog{}, err
		}
		if norm(name) == SharedEnvKey {
			shared = values
			continue
		}
		if IsReservedEnvironment(name) {
			return Catalog{}, reservedName("environment", name)
		}
		envs = append(envs, entry{name: name, values: values})
	}

	if shared != nil && len(envs) == 0 {
		return Catalog{}, diag.Newf(
			diag.ClassParse,
			`environment set defines only %q; add an environment`,
			SharedEnvKey,
		)
	}
	slices.SortFunc(envs, func(a, b entry) int { return cmpFold(a.name, b.name) })
	for i := range envs {
		envs[i].values = mergeValues(shared, envs[i].values)
	}
	return Catalog{envs: envs}, nil
}

func NewGroupedCatalog(shared map[string]string, groups []Group) (Catalog, error) {
	if len(groups) == 0 {
		return Catalog{}, diag.New(diag.ClassParse, "grouped environments require a group")
	}

	seen := newNames("group", len(groups))
	out := make([]Group, 0, len(groups))
	for _, in := range groups {
		g, err := buildGroup(in, seen)
		if err != nil {
			return Catalog{}, err
		}
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b Group) int { return cmpFold(a.Name, b.Name) })
	if err := checkCollisions(out); err != nil {
		return Catalog{}, err
	}
	return Catalog{groups: out, shared: maps.Clone(shared)}, nil
}

func buildGroup(in Group, groups *names) (Group, error) {
	name, err := groups.add(in.Name)
	if err != nil {
		return Group{}, err
	}
	switch {
	case IsReservedEnvironment(name):
		return Group{}, reservedName("group", name)
	case strings.ContainsRune(name, '='):
		return Group{}, diag.Newf(diag.ClassParse, "group name %q cannot contain '='", name)
	case len(in.Profiles) == 0:
		return Group{}, diag.Newf(diag.ClassParse, "group %q requires a profile", name)
	}

	seen := newNames("profile", len(in.Profiles))
	set := make(EnvironmentSet, len(in.Profiles))
	for raw, values := range in.Profiles {
		p, err := seen.add(raw)
		if err != nil {
			return Group{}, diag.WrapAsf(diag.ClassParse, err, "group %q", name)
		}
		if IsReservedEnvironment(p) {
			return Group{}, reservedName("profile", p)
		}
		set[p] = values
	}

	def := str.Trim(in.Default)
	if def == "" {
		if len(set) != 1 {
			return Group{}, diag.Newf(diag.ClassParse, "group %q requires %q", name, DefaultEnvKey)
		}
		for p := range set {
			def = p
		}
	} else {
		p, ok := mapName(set, def)
		if !ok {
			return Group{}, diag.Newf(
				diag.ClassParse,
				"default profile %q does not exist in group %q",
				def,
				name,
			)
		}
		def = p
	}
	return Group{Name: name, Default: def, Profiles: set}, nil
}

func (c Catalog) Grouped() bool {
	return len(c.groups) > 0
}

func (c Catalog) Empty() bool {
	return len(c.envs) == 0 && len(c.groups) == 0
}

func (c Catalog) Len() int {
	if c.Grouped() {
		return len(c.groups)
	}
	return len(c.envs)
}

func (c Catalog) Names() []string {
	names := make([]string, len(c.envs))
	for i := range c.envs {
		names[i] = c.envs[i].name
	}
	return names
}

func (c Catalog) Groups() []Group {
	return c.groups
}

func (g Group) ProfileNames() []string {
	return slices.SortedFunc(maps.Keys(g.Profiles), cmpFold)
}

func (c Catalog) findEnv(name string) (entry, bool) {
	name = str.Trim(name)
	i := slices.IndexFunc(c.envs, func(e entry) bool { return strings.EqualFold(e.name, name) })
	if i < 0 {
		return entry{}, false
	}
	return c.envs[i], true
}

func (c Catalog) findGroup(name string) (Group, bool) {
	name = str.Trim(name)
	i := slices.IndexFunc(c.groups, func(g Group) bool { return strings.EqualFold(g.Name, name) })
	if i < 0 {
		return Group{}, false
	}
	return c.groups[i], true
}

func parseCatalog(data []byte) (Catalog, error) {
	var root object
	if err := json.Unmarshal(data, &root); err != nil {
		return Catalog{}, err
	}
	if root.has(GroupsEnvKey) {
		return parseGroups(root)
	}
	return parseFlat(root)
}

// object preserves key order and duplicates, which encoding/json maps drop.
type object []field

type field struct {
	name string
	raw  json.RawMessage
}

func (o *object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return fmt.Errorf("expected object")
	}
	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return err
		}
		name, ok := tok.(string)
		if !ok {
			return fmt.Errorf("expected object key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		*o = append(*o, field{name: name, raw: raw})
	}
	if _, err := dec.Token(); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (o object) has(name string) bool {
	return slices.ContainsFunc(o, func(f field) bool { return norm(f.name) == name })
}

func parseFlat(root object) (Catalog, error) {
	set := make(EnvironmentSet, len(root))
	seen := newNames("environment", len(root))
	for _, f := range root {
		name, err := seen.add(f.name)
		if err != nil {
			return Catalog{}, err
		}
		// Flat environments are named objects. Without this check, scalar values
		// loaded as empty environments and arrays created numeric root keys.
		values, err := flattenObject(f.raw, name)
		if err != nil {
			return Catalog{}, err
		}
		set[name] = values
	}
	return NewCatalog(set)
}

func parseGroups(root object) (Catalog, error) {
	var shared map[string]string
	var raw object
	seen := newNames("schema", len(root))
	for _, f := range root {
		name, err := seen.add(f.name)
		if err != nil {
			return Catalog{}, err
		}
		switch norm(name) {
		case SharedEnvKey:
			shared, err = flattenObject(f.raw, SharedEnvKey)
			if err != nil {
				return Catalog{}, err
			}
		case GroupsEnvKey:
			if err := json.Unmarshal(f.raw, &raw); err != nil {
				return Catalog{}, diag.Newf(diag.ClassParse, "%q must be an object", GroupsEnvKey)
			}
		default:
			return Catalog{}, diag.Newf(
				diag.ClassParse,
				"grouped and flat environment definitions cannot be mixed; unexpected key %q",
				name,
			)
		}
	}

	groups := make([]Group, 0, len(raw))
	for _, f := range raw {
		var obj object
		if err := json.Unmarshal(f.raw, &obj); err != nil {
			return Catalog{}, diag.Newf(diag.ClassParse, "group %q must be an object", str.Trim(f.name))
		}
		g, err := parseGroup(str.Trim(f.name), obj)
		if err != nil {
			return Catalog{}, err
		}
		groups = append(groups, g)
	}
	return NewGroupedCatalog(shared, groups)
}

func parseGroup(name string, obj object) (Group, error) {
	g := Group{Name: name, Profiles: make(EnvironmentSet)}
	seen := newNames("profile", len(obj))
	for _, f := range obj {
		p, err := seen.add(f.name)
		if err != nil {
			return Group{}, diag.WrapAsf(diag.ClassParse, err, "group %q", name)
		}
		if norm(p) == DefaultEnvKey {
			if err := json.Unmarshal(f.raw, &g.Default); err != nil {
				return Group{}, diag.Newf(
					diag.ClassParse,
					"%q in group %q must be a string",
					DefaultEnvKey,
					name,
				)
			}
			continue
		}
		values, err := flattenObject(f.raw, p)
		if err != nil {
			return Group{}, err
		}
		g.Profiles[p] = values
	}
	return g, nil
}

func flattenObject(raw json.RawMessage, name string) (map[string]string, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
		return nil, diag.Newf(diag.ClassParse, "%q must be an object", name)
	}
	return flattenValue(obj), nil
}

func flattenValue(value any) map[string]string {
	out := make(map[string]string)
	flattenInto(out, "", value)
	return out
}

func flattenInto(out map[string]string, path string, value any) {
	switch v := value.(type) {
	case map[string]any:
		for name, child := range v {
			if name != "" {
				flattenInto(out, joinPath(path, name), child)
			}
		}
	case []any:
		for i, child := range v {
			flattenInto(out, indexPath(path, i), child)
		}
	default:
		setValue(out, path, v)
	}
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func indexPath(path string, i int) string {
	index := strconv.Itoa(i)
	if path == "" {
		return index
	}
	return path + "[" + index + "]"
}

func setValue(out map[string]string, name string, value any) {
	if name == "" {
		return
	}
	switch v := value.(type) {
	case string:
		out[name] = v
	case float64:
		out[name] = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		out[name] = strconv.FormatBool(v)
	case nil:
		out[name] = ""
	}
}

// mergeValues merges layers left to right into a new map. Variable names
// match case-insensitively and a later layer replaces the earlier spelling.
// Later layers win. Within one layer the names are applied in order, so two
// forms of one name resolve the same way on every run instead of by map order.
func mergeValues(layers ...map[string]string) map[string]string {
	var out NameMap[string]
	for _, layer := range layers {
		out.SetMap(layer)
	}
	return out.Map()
}

// Environment values are read as a plain map, and a reader that keys them
// refuses one holding two forms of a name. Collapse once per selection so every
// reader sees the same single entry.
func collapseNames(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return CollectNames(values).Map()
}

func checkCollisions(groups []Group) error {
	for i, a := range groups {
		for _, b := range groups[i+1:] {
			for ap, av := range a.Profiles {
				for bp, bv := range b.Profiles {
					if name, ok := collision(av, bv); ok {
						return diag.Newf(
							diag.ClassParse,
							"variable %q collides between %s=%q and %s=%q",
							name,
							a.Name,
							ap,
							b.Name,
							bp,
						)
					}
				}
			}
		}
	}
	return nil
}

func collision(a, b map[string]string) (string, bool) {
	keys := make(map[string]string, len(a))
	for name := range a {
		keys[strings.ToLower(name)] = name
	}
	for name := range b {
		if key, ok := keys[strings.ToLower(name)]; ok {
			return key, true
		}
	}
	return "", false
}

// names dedupes names case-insensitively and remembers the first spelling.
type names struct {
	kind string
	seen map[string]string
}

func newNames(kind string, size int) *names {
	return &names{kind: kind, seen: make(map[string]string, size)}
}

func (n *names) add(raw string) (string, error) {
	name := str.Trim(raw)
	key := norm(name)
	if key == "" {
		return "", diag.Newf(diag.ClassParse, "%s name cannot be blank", n.kind)
	}
	if prev, ok := n.seen[key]; ok {
		return "", diag.Newf(diag.ClassParse, "duplicate %s names %q and %q", n.kind, prev, name)
	}
	n.seen[key] = name
	return name, nil
}

func reservedName(kind, name string) error {
	return diag.Newf(diag.ClassParse, "%s name %q is reserved", kind, name)
}

func mapName[V any](m map[string]V, name string) (string, bool) {
	name = str.Trim(name)
	for n := range m {
		if strings.EqualFold(n, name) {
			return n, true
		}
	}
	return "", false
}

func cmpFold(a, b string) int {
	return strings.Compare(strings.ToLower(a), strings.ToLower(b))
}
