package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

type PackCheckItem struct {
	Part  string   `json:"part"` // game, dlc, mods
	Level string   `json:"level"`
	Text  string   `json:"text"`
	Names []string `json:"names,omitempty"`
}

// PackCheck compares recorded requirements with one environment. Content problems
// are supplied by verification; this comparison never walks the copies.
type PackCheck struct {
	Items    []PackCheckItem `json:"items"`
	PackGame string          `json:"packGame"`
	Game     string          `json:"game"`
}

func (c *PackCheck) Blocked() bool { return len(c.Errors()) != 0 }

func (c *PackCheck) Errors() []string {
	var texts []string
	if c != nil {
		for _, item := range c.Items {
			if item.Level == finding.Error {
				texts = append(texts, item.Text)
			}
		}
	}
	return texts
}

func installedOfficial(mods []*store.Mod) []*store.Mod {
	var out []*store.Mod
	for _, m := range mods {
		if m.Kind == source.Official && m.Status == identity.Live {
			out = append(out, m)
		}
	}
	return out
}

func (a *App) CheckPack(p *Pack, e *env.Environment, problems []PackProblem) *PackCheck {
	c := &PackCheck{PackGame: p.Game}
	add := func(part, level, text string, names []string) {
		c.Items = append(c.Items, PackCheckItem{Part: part, Level: level, Text: text, Names: names})
	}
	if e == nil {
		add("game", finding.Info, "no game selected", nil)
	} else {
		c.Game = e.GameVersion
		pv, gv := env.ParseVersion(p.Game), env.ParseVersion(e.GameVersion)
		same, known := pv.SameBuild(gv)
		switch {
		case pv.Short() != "" && gv.Short() != "" && pv.Short() != gv.Short():
			add("game", finding.Error, fmt.Sprintf("built for %s, this game is %s", pv.Full, gv.Full), nil)
		case !known:
			add("game", finding.Info, "build not recorded", nil)
		case !same:
			add("game", finding.Warning, fmt.Sprintf("built for %s, this game is %s", pv.Full, gv.Full), nil)
		}
		mods, err := a.Store.Mods(e.ID)
		if err != nil {
			add("dlc", finding.Error, "cannot read installed official content: "+err.Error(), nil)
		} else {
			installed := map[string]string{}
			for _, m := range installedOfficial(mods) {
				installed[strings.ToLower(m.PackageID)] = m.DisplayName()
			}
			wanted := map[string]bool{}
			var missing, extra []string
			for _, entry := range p.Entries {
				if !entry.Official {
					continue
				}
				pkg := strings.ToLower(entry.PackageID)
				if wanted[pkg] {
					continue
				}
				wanted[pkg] = true
				if _, ok := installed[pkg]; !ok {
					name := entry.Name
					if name == "" {
						name = entry.PackageID
					}
					missing = append(missing, name)
				}
			}
			for pkg, name := range installed {
				if pkg != "ludeon.rimworld" && !wanted[pkg] {
					extra = append(extra, name)
				}
			}
			sort.Strings(missing)
			sort.Strings(extra)
			if len(missing) > 0 {
				add("dlc", finding.Error, "missing DLC: "+strings.Join(missing, ", "), missing)
			}
			if len(extra) > 0 {
				add("dlc", finding.Info, "installed DLC not used by the pack: "+strings.Join(extra, ", "), extra)
			}
		}
	}
	for _, problem := range problems {
		name := problem.Name
		if name == "" {
			name = problem.Folder
		}
		if name == "" {
			name = problem.PackageID
		}
		add("mods", finding.Error, fmt.Sprintf("copy could not be verified: %s (%s)", name, problem.Why), []string{name})
	}
	return c
}

func (c *PackCheck) missingMods(entries []PackEntry) {
	if len(entries) == 0 {
		return
	}
	var names []string
	for _, e := range entries {
		name := e.Name
		if name == "" {
			name = e.PackageID
		}
		names = append(names, name)
	}
	c.Items = append(c.Items, PackCheckItem{Part: "mods", Level: finding.Warning, Text: "mods not installed: " + strings.Join(names, ", "), Names: names})
}
