package directive

import "testing"

func TestSpecsUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[Name]Name)
	for _, spec := range Specs() {
		if spec.Name == "" {
			t.Fatal("empty canonical directive name")
		}
		names := append([]Name{spec.Name}, spec.Aliases...)
		for _, name := range names {
			if owner, ok := seen[name]; ok {
				t.Fatalf("directive %q belongs to both %q and %q", name, owner, spec.Name)
			}
			seen[name] = spec.Name
		}
	}
}

// Every spelling in the table has to resolve, otherwise completion offers a
// directive that the parser cannot canonicalize.
func TestLookupCoversEverySpelling(t *testing.T) {
	t.Parallel()

	for _, spec := range Specs() {
		for _, name := range append([]Name{spec.Name}, spec.Aliases...) {
			got, ok := Lookup(name)
			if !ok {
				t.Fatalf("Lookup(%q) missing", name)
			}
			if got.Name != spec.Name {
				t.Fatalf("Lookup(%q).Name = %q, want %q", name, got.Name, spec.Name)
			}
			if canon := name.Canonical(); canon != spec.Name {
				t.Fatalf("%q.Canonical() = %q, want %q", name, canon, spec.Name)
			}
		}
	}
}

func TestCanonical(t *testing.T) {
	t.Parallel()

	if got := SkipIf.Canonical(); got != When {
		t.Fatalf("%q.Canonical() = %q, want %q", SkipIf, got, When)
	}
	if got := Name("future").Canonical(); got != "future" {
		t.Fatalf("Name(%q).Canonical() = %q, want unchanged", "future", got)
	}
	if _, ok := Lookup("future"); ok {
		t.Fatal("Lookup(\"future\") ok = true, want false")
	}
}

func TestNameTag(t *testing.T) {
	t.Parallel()

	if got := Assert.Tag(); got != "@assert" {
		t.Fatalf("Assert.Tag() = %q, want %q", got, "@assert")
	}
	if got := Assert.Comment(); got != "# @assert" {
		t.Fatalf("Assert.Comment() = %q, want %q", got, "# @assert")
	}
}
