// Spike 5: how RimWorld encodes workshop-vs-local duplicates in ModsConfig.xml.
// Read-only: inspects ModsConfig.xml and the mod folders, never writes.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rimarcho/rimarcho/spikes/internal/rwenv"
)

func main() {
	env, err := rwenv.Detect()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	mc, err := rwenv.ReadModsConfig(env.ModsConfigPath())
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	reg := env.Scan()
	fmt.Printf("game: %s\nModsConfig version: %s, active: %d\n", env.GameDir, mc.Version, len(mc.ActiveMods))
	fmt.Printf("scanned mods: %d (rejected duplicates: %d)\n", len(reg.All), len(reg.Rejected))

	steamEntries := 0
	for _, e := range mc.ActiveMods {
		if strings.HasSuffix(e, "_steam") {
			steamEntries++
			fmt.Printf("  _steam entry: %s\n", e)
		}
	}
	fmt.Printf("_steam entries in activeMods: %d\n", steamEntries)

	active := map[string]bool{}
	for _, e := range mc.ActiveMods {
		active[e] = true
	}
	fmt.Println("\npackageIds present both locally and on workshop:")
	n := 0
	for _, m := range reg.All {
		if !m.SteamPostfix {
			continue
		}
		n++
		local := reg.ByID[m.PackageID]
		state := "neither active"
		switch {
		case active[m.PackageID] && active[m.GameID()]:
			state = "BOTH entries active"
		case active[m.PackageID]:
			state = "local active (plain id)"
		case active[m.GameID()]:
			state = "workshop active (_steam)"
		}
		fmt.Printf("  %-45s local=%s workshop=%s  → %s\n", m.PackageID, rel(env, local), filepath.Base(m.Dir), state)
	}
	fmt.Printf("total: %d\n", n)
	fmt.Println("\nrejected (same packageId, same source kind — game logs an error and ignores):")
	for _, m := range reg.Rejected {
		fmt.Printf("  %-45s %s %s\n", m.PackageID, m.Source, rel(env, m))
	}
	missing := 0
	for _, e := range mc.ActiveMods {
		if reg.Resolve(e) == nil {
			missing++
			fmt.Printf("  unresolved active entry: %s\n", e)
		}
	}
	fmt.Printf("unresolved active entries: %d\n", missing)
}

func rel(env *rwenv.Env, m *rwenv.Mod) string {
	if m == nil {
		return "-"
	}
	if r, err := filepath.Rel(env.GameDir, m.Dir); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}
	return filepath.Base(m.Dir)
}
