package scripts

import "maps"

// StreamInfo carries streaming session data for script consumption.
type StreamInfo struct {
	Kind    string
	Summary map[string]any
	Events  []map[string]any
}

func (info *StreamInfo) Clone() *StreamInfo {
	if info == nil {
		return nil
	}
	clone := &StreamInfo{Kind: info.Kind}
	if len(info.Summary) > 0 {
		clone.Summary = make(map[string]any, len(info.Summary))
		maps.Copy(clone.Summary, info.Summary)
	}
	if len(info.Events) > 0 {
		clone.Events = make([]map[string]any, len(info.Events))
		for i, evt := range info.Events {
			if evt == nil {
				continue
			}

			copyEvt := make(map[string]any, len(evt))
			maps.Copy(copyEvt, evt)
			clone.Events[i] = copyEvt
		}
	}
	return clone
}
