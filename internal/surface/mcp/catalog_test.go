package mcp_test

import (
	"reflect"
	"strings"
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

func TestClipOutput(t *testing.T) {
	short := "a\nb\n"
	if mcp.ClipOutput(short) != short {
		t.Error("short output changed")
	}
	long := strings.Repeat("0123456789abcdef0123456789abcdef\n", 4000) // 132 KB
	got := mcp.ClipOutput(long)
	if len(got) > mcp.MaxOutput+200 || !strings.Contains(got, "output clipped at") || !strings.Contains(got, "narrow the call") {
		t.Errorf("clipped to %d bytes, tail %q", len(got), got[len(got)-150:])
	}
}

func TestFilesSchemaAndArgv(t *testing.T) {
	var search *mcp.ToolDoc
	for _, tool := range mcp.Catalog() {
		if tool.Name == "search_files" {
			search = &tool
			break
		}
	}
	if search == nil || !search.ReadOnly {
		t.Fatal("search_files must remain read-only")
	}
	names := map[string]string{}
	for _, p := range search.Params {
		names[p.Name] = p.Type
	}
	for _, name := range []string{"pattern", "mod", "kind", "game", "mode", "game_version"} {
		if names[name] != "string" {
			t.Errorf("%s: %s", name, names[name])
		}
	}
	for _, name := range []string{"def", "regex", "word", "case_sensitive", "languages", "all_versions", "all"} {
		if names[name] != "boolean" {
			t.Errorf("%s: %s", name, names[name])
		}
	}
	for _, name := range []string{"mods", "lang"} {
		if !strings.Contains(names[name], "string") {
			t.Errorf("%s: %s", name, names[name])
		}
	}
	for _, name := range []string{"page", "limit", "per_file"} {
		if names[name] != "integer" {
			t.Errorf("%s: %s", name, names[name])
		}
	}
	got, err := mcp.ArgvFor("search_files", []byte(`{"pattern":"Target","mod":"#a","mods":["#b","#c"],"def":true,"regex":true,"word":true,"case_sensitive":true,"kind":"all","languages":true,"lang":["English","French"],"all_versions":true,"game":"without","mode":"files","page":2,"limit":7,"per_file":3,"all":true,"game_version":"1.5"}`))
	want := []string{"files", "search", "--mod", "#a", "--mod", "#b", "--mod", "#c", "--def", "--regex", "--word", "--case", "--kind", "all", "--languages", "--lang", "English", "--lang", "French", "--all-versions", "--game", "without", "--mode", "files", "--page", "2", "--limit", "7", "--per-file", "3", "--all", "--game-version", "1.5", "Target"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q (%v), want %q", got, err, want)
	}
	if got := mcp.FilesArgv(mcp.FilesIn{Pattern: "--def"}); !reflect.DeepEqual(got, []string{"files", "search", "--", "--def"}) {
		t.Fatal(got)
	}
	for _, text := range []string{"exact copies", "load order", "Defs/Patches/About", "translations are excluded", "def=true", "per_file", "24 KB", "Gun_Revolver"} {
		if !strings.Contains(search.Description, text) {
			t.Errorf("description missing %q", text)
		}
	}
}
