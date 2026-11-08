package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// StrList is a list argument (mod references, tags, names). Models often send one as a
// string: a JSON array written out ("[\"#vyzv\"]") or a single bare value ("#vyzv").
// Both are accepted besides a real array, so a call is not rejected for its spelling
// (14 §2, h7 B2). A string that is not a JSON array is one item; a number is its digits.
type StrList []string

// UnmarshalJSON accepts null, an array of strings or numbers, or a string.
func (l *StrList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*l = nil
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		switch {
		case s == "":
			*l = nil
			return nil
		case s[0] == '[':
			if out, err := decodeStrings([]byte(s)); err == nil {
				*l = out
				return nil
			}
		}
		*l = StrList{s}
		return nil
	}
	out, err := decodeStrings(b)
	if err != nil {
		return err
	}
	*l = out
	return nil
}

func decodeStrings(b []byte) (StrList, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var raw []any
	if err := d.Decode(&raw); err != nil {
		return nil, err
	}
	out := make(StrList, 0, len(raw))
	for _, v := range raw {
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case json.Number:
			out = append(out, x.String())
		default:
			return nil, fmt.Errorf("a list item must be a string, not %v", v)
		}
	}
	return out, nil
}

// inputSchema is the argument schema of a tool: the SDK's own, with a StrList property
// also allowing a string (the SDK validates the call against it before our decoder runs).
func inputSchema[In any]() (*jsonschema.Schema, error) {
	return jsonschema.For[In](&jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[StrList](): {Types: []string{"null", "array", "string"}, Items: &jsonschema.Schema{Type: "string"}},
	}})
}
