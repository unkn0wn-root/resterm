package restfile

import (
	"fmt"
	"maps"
)

func (a *AuthSpec) Clone() *AuthSpec {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Params = cloneAuthParams(a.Params)
	return &cp
}

func (a *AuthSpec) Origin() string {
	if a == nil {
		return ""
	}
	return origin(a.SourcePath, a.Line)
}

func origin(path string, line int) string {
	switch {
	case path != "" && line > 0:
		return fmt.Sprintf("%s:%d", path, line)
	case path != "":
		return path
	case line > 0:
		return fmt.Sprintf("line %d", line)
	default:
		return ""
	}
}

func cloneAuthParams(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}
