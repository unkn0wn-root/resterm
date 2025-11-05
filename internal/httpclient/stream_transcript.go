package httpclient

import "encoding/json"

// DecodeSSETranscript unmarshals SSE transcript JSON for CLI replay.
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

// DecodeWebSocketTranscript unmarshals websocket transcript JSON blobs.
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
