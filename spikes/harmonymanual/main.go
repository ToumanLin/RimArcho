// Spike: run the manual Harmony.Patch scan (internal/harmony/manual.go) over DLLs or mod folders
// and print, per DLL, the Harmony.Patch calls found, resolved, and why the rest were not.
//
//	go run ./spikes/harmonymanual [-v] <dll or folder> …
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/harmony"
)

func main() {
	verbose := flag.Bool("v", false, "list the patches")
	flag.Parse()
	var dlls []string
	for _, p := range flag.Args() {
		filepath.WalkDir(p, func(f string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(f), ".dll") && !harmony.Skipped(filepath.Base(f), harmony.DefaultSkip) {
				dlls = append(dlls, f)
			}
			return nil
		})
	}
	tot := map[string]int{}
	var calls, resolved, patches, manual, unres int
	for _, f := range dlls {
		a, err := harmony.Read(f, false)
		if err != nil {
			fmt.Printf("%s: %v\n", f, err)
			continue
		}
		if !a.Manual {
			continue
		}
		nm, nu := 0, 0
		for _, p := range a.Patches {
			if p.Manual {
				nm++
				if p.Dynamic {
					nu++
				}
			}
		}
		calls += a.ManualCalls
		resolved += a.ManualResolved
		patches += len(a.Patches)
		manual += nm
		unres += nu
		var rs []string
		for r, n := range a.Unresolved {
			tot[r] += n
			rs = append(rs, fmt.Sprintf("%s %d", r, n))
		}
		sort.Strings(rs)
		fmt.Printf("%s: calls %d resolved %d; patches %d (manual %d, target unknown %d); unresolved: %s\n", f, a.ManualCalls, a.ManualResolved, len(a.Patches), nm, nu, strings.Join(rs, ", "))
		if *verbose {
			for _, p := range a.Patches {
				if p.Manual {
					fmt.Printf("   %s %s <- %s.%s\n", p.Kind, p.Target(), p.PatchType, p.PatchMethod)
				}
			}
		}
	}
	type kv struct {
		k string
		n int
	}
	var all []kv
	for k, n := range tot {
		all = append(all, kv{k, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	fmt.Printf("\nTOTAL calls %d resolved %d; patches %d (manual %d, target unknown %d)\n", calls, resolved, patches, manual, unres)
	for _, x := range all {
		fmt.Printf("  %6d  %s\n", x.n, x.k)
	}
}
