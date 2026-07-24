package restfile

import "maps"

func (a *AuthSpec) Clone() *AuthSpec {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Params = cloneAuthParams(a.Params)
	return &cp
}

func cloneAuthParams(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}
