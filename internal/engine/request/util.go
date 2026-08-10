package request

import "maps"

func copyBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	return append([]byte(nil), src...)
}

func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	maps.Copy(dst, src)
	return dst
}

func mergeStringMaps(xs ...map[string]string) map[string]string {
	size := 0
	for _, x := range xs {
		size += len(x)
	}
	if size == 0 {
		return nil
	}

	out := make(map[string]string, size)
	for _, x := range xs {
		maps.Copy(out, x)
	}
	return out
}
