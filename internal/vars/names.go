package vars

// NameKey returns the case-insensitive, whitespace-trimmed identity of name.
func NameKey(name string) string { return norm(name) }

// Upsert writes value under name and drops any other form of that name, so a
// map keyed by authored names never holds two forms of one variable. Callers
// that write in order therefore get last-write-wins, which a plain map
// assignment cannot give them.
func Upsert(m map[string]string, name, value string) {
	key := NameKey(name)
	for cur := range m {
		if cur != name && NameKey(cur) == key {
			delete(m, cur)
		}
	}
	m[name] = value
}

// Merge folds src into dst with Upsert semantics, so a later source wins even
// when it uses a different form of a name dst already holds. src itself must
// hold one form per name, which is what building it through Upsert guarantees.
func Merge(dst, src map[string]string) {
	for name, value := range src {
		Upsert(dst, name, value)
	}
}
