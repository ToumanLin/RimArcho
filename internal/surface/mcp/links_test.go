package mcp_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestLinkToolsConfirmationAndAnnotations(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	link := filepath.Join(e.Game, "Mods", "_Archo_unproven")
	e.Junction(link, filepath.Join(e.Root, "missing"))
	cs := connect(t, e)
	listed, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, tool := range listed.Tools {
		switch tool.Name {
		case "check_links":
			if !tool.Annotations.ReadOnlyHint {
				t.Fatal("check is not read only")
			}
			found++
		case "repair_links":
			if tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
				t.Fatal("repair not destructive")
			}
			found++
		}
	}
	if found != 2 {
		t.Fatal("missing link tools")
	}
	out, isErr := call(t, cs, "check_links", map[string]any{"all": true})
	if !isErr || !json.Valid([]byte(out)) || !source.IsLink(link) {
		t.Fatalf("check: %s", out)
	}
	for _, args := range []map[string]any{
		{"confirm": false, "include": []string{link}},
		{"confirm": true},
		{"confirm": false, "dry_run": true, "include": []string{link}},
	} {
		call(t, cs, "repair_links", args)
		if !source.IsLink(link) {
			t.Fatal("removed without explicit confirmation and inclusion")
		}
	}
	out, isErr = call(t, cs, "repair_links", map[string]any{"confirm": true, "include": []string{link}})
	if isErr || !json.Valid([]byte(out)) {
		t.Fatalf("confirmed repair: %s", out)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("confirmed link remains")
	}
}
