package scripts

// StreamInfo carries streaming session data for script consumption.
type StreamInfo struct {
	Kind    string
	Summary map[string]any
	Events  []map[string]any
}

// Clone deep copies the summary and events so script runtimes can mutate data
// without affecting callers.
func (info *StreamInfo) Clone() *StreamInfo {
	if info == nil {
		return nil
	}
	clone := &StreamInfo{Kind: info.Kind}
	if len(info.Summary) > 0 {
		clone.Summary = make(map[string]any, len(info.Summary))
		for k, v := range info.Summary {
			clone.Summary[k] = v
		}
	}
	if len(info.Events) > 0 {
		clone.Events = make([]map[string]any, len(info.Events))
		for i, evt := range info.Events {
			if evt == nil {
				continue
			}

			copyEvt := make(map[string]any, len(evt))
			for k, v := range evt {
				copyEvt[k] = v
			}
			clone.Events[i] = copyEvt
		}
	}
	return clone
}
