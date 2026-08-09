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

func parseJSONObject(raw string) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	if err := restfile.CheckMockJSONKeys([]byte(raw)); err != nil {
		return nil, err
	}
	return obj, nil
}

// The option lexer removes one layer of quotes, so a JSON string reaches this
// function as a bare word. Other invalid values keep the decoder's original
// error because the quoting hint would not help them.
func jsonValueError(raw string, err error) error {
	if err == nil || raw == "" || strings.ContainsAny(raw, "{}[]\",:'\\") {
		return err
	}
	quoted := strconv.Quote(raw)
	if quoted != `"`+raw+`"` {
		return err
	}
	return fmt.Errorf("%s is not a JSON value. Write json='%s' to match a body that is a JSON string", raw, quoted)
}

func compactJSON(raw string) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err != nil {
		return nil, err
	}
	if err := restfile.CheckMockJSONKeys(compact.Bytes()); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
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
