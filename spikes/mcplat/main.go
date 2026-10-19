// Spike: MCP single-query latency (01 §5.2, target ≤ 500 ms with the index built). Starts
// `archo mcp` once and times each read-only tool call (5 runs, median and max).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	exe := flag.String("archo", `bin\archo.exe`, "archo executable")
	ref := flag.String("ref", "pkg:brrainz.harmony", "a mod reference")
	flag.Parse()
	ctx := context.Background()
	c := sdk.NewClient(&sdk.Implementation{Name: "mcplat", Version: "0"}, nil)
	t0 := time.Now()
	cs, err := c.Connect(ctx, &sdk.CommandTransport{Command: exec.Command(*exe, "mcp")}, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer cs.Close()
	fmt.Printf("connect (process start + initialize): %d ms\n", time.Since(t0).Milliseconds())
	calls := []struct {
		name string
		args map[string]any
	}{
		{"status", nil},
		{"find_mods", map[string]any{"query": "combat"}},
		{"get_mod", map[string]any{"ref": *ref}},
		{"get_def", map[string]any{"def": "ThingDef/Gun_Revolver"}},
		{"get_patches", map[string]any{"failed": true}},
		{"get_harmony", map[string]any{"target": "Verse.Pawn.Tick"}},
		{"list_problems", nil},
		{"list_changes", nil},
		{"analyze_log", nil},
		{"analyze_log", map[string]any{"cluster": "E15"}},
		{"search_files", map[string]any{"pattern": "Gun_Revolver"}},
	}
	for _, cl := range calls {
		var ds []time.Duration
		for i := 0; i < 5; i++ {
			t := time.Now()
			res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: cl.name, Arguments: cl.args})
			if err != nil {
				log.Fatal(err)
			}
			_ = res
			ds = append(ds, time.Since(t))
		}
		first := ds[0]
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		fmt.Printf("%-14s %-40v first %4d ms  median %4d ms  max %4d ms\n", cl.name, cl.args, first.Milliseconds(), ds[2].Milliseconds(), ds[4].Milliseconds())
	}
}
