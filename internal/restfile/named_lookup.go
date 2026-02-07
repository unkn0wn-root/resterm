package restfile

import "strings"

func LookupNamedScoped[T any, S comparable](
	xs []T,
	n string,
	sc S,
	sf func(T) S,
	nf func(T) string,
) (*T, bool) {
	nk := namedKey(n)
	if nk == "" {
		return nil, false
	}
	for i := range xs {
		v := &xs[i]
		if sf(*v) != sc {
			continue
		}
		if namedKey(nf(*v)) == nk {
			return v, true
		}
	}
	return nil, false
}

func ResolveNamedScoped[T any, S comparable](
	fs []T,
	gs []T,
	n string,
	fsc S,
	gsc S,
	sf func(T) S,
	nf func(T) string,
) (*T, bool) {
	if v, ok := LookupNamedScoped(fs, n, fsc, sf, nf); ok {
		return v, true
	}
	return LookupNamedScoped(gs, n, gsc, sf, nf)
}

func namedKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
