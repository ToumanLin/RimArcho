package mcp_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// h7 B2: a list argument may arrive as a JSON-array string or a bare string.
func TestListArgumentsAcceptStrings(t *testing.T) {
	t.Parallel()
	want := []string{"source", "search", "X", "--in", "#vyzv", "--budget", "10"}
	for name, in := range map[string]string{
		"array":         `["#vyzv"]`,
		"array string":  `"[\"#vyzv\"]"`,
		"padded string": `"  [ \"#vyzv\" ]"`,
		"bare string":   `"#vyzv"`,
	} {
		got, err := mcp.ArgvFor("read_source", []byte(`{"search":"X","in":`+in+`}`))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q, %v; want %q", name, got, err, want)
		}
	}
	two, err := mcp.ArgvFor("read_source", []byte(`{"search":"X","in":"[\"#a1\", \"#b2\"]"}`))
	if want := []string{"source", "search", "X", "--in", "#a1", "--in", "#b2", "--budget", "10"}; err != nil || !reflect.DeepEqual(two, want) {
		t.Errorf("two refs: %q, %v", two, err)
	}
	none, err := mcp.ArgvFor("read_source", []byte(`{"search":"X","in":null}`))
	if want := []string{"source", "search", "X", "--implicated", "--budget", "10"}; err != nil || !reflect.DeepEqual(none, want) {
		t.Errorf("null: %q, %v", none, err)
	}
	empty, err := mcp.ArgvFor("read_source", []byte(`{"search":"X","in":""}`))
	if err != nil || !reflect.DeepEqual(empty, none) {
		t.Errorf("empty string: %q, %v", empty, err)
	}
	// Other tools with a list: numbers are their digits, anything else is refused.
	if got, err := mcp.ArgvFor("edit_loadout", []byte(`{"action":"enable","refs":"pkg:a.b"}`)); err != nil || !reflect.DeepEqual(got, []string{"enable", "pkg:a.b"}) {
		t.Errorf("edit_loadout bare ref: %q, %v", got, err)
	}
	if _, err := mcp.ArgvFor("read_source", []byte(`{"search":"X","in":[{"a":1}]}`)); err == nil {
		t.Error("an object in a list was accepted")
	}
}

// The SDK validates a call against the tool's schema before our decoder runs: the schema
// must allow a string too, or the call is refused ("in: must be either null or array").
func TestListArgumentSchemaAllowsStrings(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.HarmonyLogFixture()
	cs := connect(t, e)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "read_source" {
			continue
		}
		b, _ := json.Marshal(tl.InputSchema)
		var sc struct {
			Properties map[string]struct {
				Type  any `json:"type"`
				Items struct {
					Type string `json:"type"`
				} `json:"items"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(b, &sc); err != nil {
			t.Fatal(err)
		}
		in := sc.Properties["in"]
		types, _ := json.Marshal(in.Type)
		if !strings.Contains(string(types), `"array"`) || !strings.Contains(string(types), `"string"`) || in.Items.Type != "string" {
			t.Fatalf("schema of in: %s", b)
		}
	}
	for _, in := range []any{[]string{"nobody"}, `["nobody"]`, "nobody"} {
		text, isErr := call(t, cs, "read_source", map[string]any{"search": "AllComps.Remove(", "in": in})
		if strings.Contains(text, "validating") || strings.Contains(text, "must be") {
			t.Errorf("in=%#v refused by the schema: %s", in, text)
		}
		if !isErr || !strings.Contains(text, "nobody") {
			t.Errorf("in=%#v: want the reference error for the unknown mod, got (%v) %s", in, isErr, text)
		}
	}
}
