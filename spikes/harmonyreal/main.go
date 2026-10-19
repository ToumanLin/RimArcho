// Spike: run the M4 Harmony reader over every DLL of the active mods (read-only) and
// print totals, timings and samples.
//
//	go run ./spikes/harmonyreal -home %APPDATA%\RimArcho
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/harmony"
)

func main() {
	home := flag.String("home", "", "RimArcho data directory")
	flag.Parse()
	a, err := app.Open(*home)
	if err != nil {
		panic(err)
	}
	st, err := a.Loadout()
	if err != nil {
		panic(err)
	}
	var dlls []string
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil {
			continue
		}
		filepath.WalkDir(st.Ix.ModDir(m), func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".dll") && strings.Contains(strings.ToLower(p), "assemblies") {
				dlls = append(dlls, p)
			}
			return nil
		})
	}
	t0 := time.Now()
	var types, patches, manual, errs, attrErr, dyn, skips, ids, read int
	kinds := map[string]int{}
	var samples []string
	for _, p := range dlls {
		skip := harmony.Skipped(filepath.Base(p), harmony.DefaultSkip)
		r, err := harmony.Read(p, skip)
		if err != nil {
			errs++
			continue
		}
		read++
		types += len(r.Types)
		patches += len(r.Patches)
		attrErr += r.AttrError
		ids += len(r.IDs)
		if r.Manual {
			manual++
		}
		for _, x := range r.Patches {
			kinds[x.Kind]++
			if x.Dynamic {
				dyn++
			}
			if x.Skips {
				skips++
			}
			if len(samples) < 12 && strings.Contains(x.Target(), "Pawn.Tick") {
				samples = append(samples, fmt.Sprintf("%s  %s.%s %s → %s", filepath.Base(p), x.PatchType, x.PatchMethod, x.Kind, x.Target()))
			}
		}
	}
	fmt.Printf("%d DLL files, %d read, %d errors, %v\n", len(dlls), read, errs, time.Since(t0).Round(time.Millisecond))
	fmt.Printf("types %d, patches %d (%v), dynamic %d, bool prefixes %d, attr decode errors %d, manual-patch assemblies %d, id candidates %d\n",
		types, patches, kinds, dyn, skips, attrErr, manual, ids)
	for _, s := range samples {
		fmt.Println("  " + s)
	}
}
