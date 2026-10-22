package mcp_test

import (
	"reflect"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
)

func TestCatalogAndArgvFor(t *testing.T) {
	cat := mcp.Catalog()
	if len(cat) != len(mcp.Tools()) {
		t.Fatalf("catalog has %d tools, server %d", len(cat), len(mcp.Tools()))
	}
	var logDoc *mcp.ToolDoc
	for i := range cat {
		if cat[i].Name == "analyze_log" {
			logDoc = &cat[i]
		}
	}
	if logDoc == nil || !logDoc.ReadOnly || logDoc.Description == "" {
		t.Fatalf("analyze_log missing or wrong: %+v", logDoc)
	}
	names := map[string]string{}
	for _, p := range logDoc.Params {
		names[p.Name] = p.Type
		if p.Name == "env" {
			t.Error("env parameter should be left out")
		}
	}
	if names["cluster"] != "string" || names["all"] != "boolean" {
		t.Errorf("params: %v", names)
	}

	cases := []struct {
		tool, args string
		want       []string
	}{
		{"analyze_log", `{"cluster":"E3"}`, []string{"log", "--cluster", "E3"}},
		{"get_patches", `{"failed":true,"mod":"pkg:a.b"}`, []string{"patches", "--mod", "pkg:a.b", "--failed"}},
		{"read_source", `{"search":"AllComps.Remove(","in":["#a1"]}`, []string{"source", "search", "AllComps.Remove(", "--in", "#a1", "--budget", "10"}},
		{"status", ``, []string{"status"}},
	}
	for _, c := range cases {
		got, err := mcp.ArgvFor(c.tool, []byte(c.args))
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s = %q, %v; want %q", c.tool, c.args, got, err, c.want)
		}
	}
	if _, err := mcp.ArgvFor("analyze_log", []byte(`{"clustr":"E3"}`)); err == nil {
		t.Error("unknown argument accepted")
	}
	if _, err := mcp.ArgvFor("nope", nil); err == nil {
		t.Error("unknown tool accepted")
	}
}
