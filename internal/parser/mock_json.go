package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/util"
)

// parseMockRules visits keys in sorted order. Map order would report a different
// broken rule on each parse, and the editor reparses on every keystroke.
func parseMockRules[T restfile.MockQueryRule | restfile.MockHeaderRule](
	raw string,
) (map[string]T, error) {
	fields, err := parseJSONObject(raw)
	if err != nil {
		return nil, err
	}
	out := make(map[string]T, len(fields))
	for _, name := range util.SortedKeys(fields) {
		var rule T
		if err := json.Unmarshal(fields[name], &rule); err != nil {
			return nil, fmt.Errorf("matcher for %q: %w", name, err)
		}
		out[name] = rule
	}
	return out, nil
}

func parseJSONObject(raw string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	if key, dup := duplicateJSONKey([]byte(raw)); dup {
		return nil, fmt.Errorf("field %q is repeated", key)
	}
	return obj, nil
}

func compactJSON(raw string) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err != nil {
		return nil, err
	}
	if key, dup := duplicateJSONKey(compact.Bytes()); dup {
		return nil, fmt.Errorf("field %q is repeated", key)
	}
	return compact.Bytes(), nil
}

// encoding/json keeps the last duplicate key, which could silently remove a
// match condition.
func duplicateJSONKey(raw []byte) (string, bool) {
	key, dup, _ := scanJSONKeys(json.NewDecoder(bytes.NewReader(raw)))
	return key, dup
}

func scanJSONKeys(dec *json.Decoder) (string, bool, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", false, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return "", false, nil
	}

	seen := map[string]bool{}
	for dec.More() {
		if delim == '{' {
			name, err := dec.Token()
			if err != nil {
				return "", false, err
			}
			key, _ := name.(string)
			if seen[key] {
				return key, true, nil
			}
			seen[key] = true
		}
		if key, dup, err := scanJSONKeys(dec); dup || err != nil {
			return key, dup, err
		}
	}
	_, err = dec.Token()
	return "", false, err
}

func mergeMockJSON(dst, src []byte) ([]byte, error) {
	into, ok := mockJSONFields(dst)
	from, alsoObject := mockJSONFields(src)
	if !ok || !alsoObject {
		return nil, errors.New("can only be repeated when every declaration is a JSON object")
	}

	var dup []string
	for _, key := range util.SortedKeys(from) {
		if _, seen := into[key]; seen {
			dup = append(dup, strconv.Quote(key))
			continue
		}
		into[key] = from[key]
	}
	switch len(dup) {
	case 0:
		return encodeMockJSON(into)
	case 1:
		return nil, fmt.Errorf("field %s is repeated", dup[0])
	default:
		return nil, fmt.Errorf("fields %s are repeated", strings.Join(dup, ", "))
	}
}

func mockJSONFields(raw []byte) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || raw[0] != '{' {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, false
	}
	return fields, true
}

// Sorting gives multiline and repeated matchers one stored form.
func sortMockJSONFields(compact []byte) []byte {
	fields, ok := mockJSONFields(compact)
	if !ok {
		return compact
	}
	sorted, err := encodeMockJSON(fields)
	if err != nil {
		return compact
	}
	return sorted
}

// Matchers are request data, so HTML escaping would change their stored form.
func encodeMockJSON(fields map[string]json.RawMessage) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(fields); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func compactJSONObject(raw string) ([]byte, error) {
	compact, err := compactJSON(raw)
	if err != nil {
		return nil, err
	}
	if len(compact) == 0 || compact[0] != '{' {
		return nil, errors.New("must be a JSON object")
	}
	return compact, nil
}
