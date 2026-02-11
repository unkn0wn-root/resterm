package sqlite

import "encoding/json"

func enc[T any](v T) ([]byte, error) {
	return json.Marshal(v)
}

func dec[T any](b []byte) (T, error) {
	var v T
	if len(b) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return v, err
	}
	return v, nil
}
