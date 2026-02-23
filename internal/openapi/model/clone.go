package model

import "github.com/unkn0wn-root/resterm/internal/util"

func CloneStrs(src []string) []string {
	return util.CloneSlice(src)
}

func SchemaTypesFromStrings(src []string) []SchemaType {
	if len(src) == 0 {
		return nil
	}
	out := make([]SchemaType, 0, len(src))
	for _, s := range src {
		t := normalizeSchemaType(SchemaType(s))
		if t == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}
