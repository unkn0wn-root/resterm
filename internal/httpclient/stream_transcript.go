package httpclient

import "encoding/json"

func DecodeSSETranscript(data []byte) (*SSETranscript, error) {
	if len(data) == 0 {
		return &SSETranscript{}, nil
	}
	var transcript SSETranscript
	if err := json.Unmarshal(data, &transcript); err != nil {
		return nil, err
	}
	return &transcript, nil
}

func DecodeWebSocketTranscript(data []byte) (*WebSocketTranscript, error) {
	if len(data) == 0 {
		return &WebSocketTranscript{}, nil
	}
	var transcript WebSocketTranscript
	if err := json.Unmarshal(data, &transcript); err != nil {
		return nil, err
	}
	return &transcript, nil
}
