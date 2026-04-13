package ui

import "github.com/unkn0wn-root/resterm/internal/scripts"

func cloneStreamInfo(info *scripts.StreamInfo) *scripts.StreamInfo {
	if info == nil {
		return nil
	}
	return info.Clone()
}
