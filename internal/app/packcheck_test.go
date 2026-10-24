package app_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func checkItem(c *app.PackCheck, part, level string) *app.PackCheckItem {
	for i := range c.Items {
		if c.Items[i].Part == part && c.Items[i].Level == level {
			return &c.Items[i]
		}
	}
	return nil
}

func TestCheckPack(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("pack", "build", "checked")
	game, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	game.GameVersion = "1.6.4871 rev590"
	if err := e.App.Store.UpdateEnv(game); err != nil {
		t.Fatal(err)
	}
	base, err := e.App.ReadPack("checked")
	if err != nil {
		t.Fatal(err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	for _, tc := range []struct {
		name, version, level string
		dlc, blocked         bool
	}{
		{"different-build", "1.6.4633 rev1261", finding.Warning, false, false},
		{"different-revision", "1.6.4871 rev591", finding.Warning, false, false},
		{"old-record", "1.6", finding.Info, false, false},
		{"missing-biotech", "1.6.4871 rev590", "", true, true},
		{"different-game", "1.5.4633 rev1261", finding.Error, false, true},
		{"same-build", "1.6.4871 rev590", "", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := *base
			p.Game = tc.version
			p.Entries = append([]app.PackEntry(nil), base.Entries...)
			if tc.dlc {
				p.Entries = append(p.Entries, app.PackEntry{PackageID: "ludeon.rimworld.biotech", Name: "Biotech", Official: true})
			}
			check := e.App.CheckPack(&p, game, nil)
			if check.Blocked() != tc.blocked {
				t.Fatalf("blocked: %+v", check)
			}
			if tc.level != "" && checkItem(check, "game", tc.level) == nil {
				t.Fatalf("game comparison: %+v", check)
			}
			if tc.dlc {
				item := checkItem(check, "dlc", finding.Error)
				if item == nil || strings.Join(item.Names, ",") != "Biotech" {
					t.Fatalf("DLC: %+v", check)
				}
			}
			b, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			e.WriteFile(filepath.Join(e.App.PacksDir(), base.Name, "pack.json"), string(b))
			launch, err := e.App.LaunchPack(base.Name, true)
			if tc.blocked {
				if !errors.Is(err, app.ErrLaunchBlocked) {
					t.Fatalf("launch refusal: %v", err)
				}
				if tc.dlc && !strings.Contains(err.Error(), "Biotech") {
					t.Fatal(err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if launch.Check == nil || launch.Check.Blocked() {
					t.Fatalf("launch check: %+v", launch)
				}
				if tc.level != "" && checkItem(launch.Check, "game", tc.level) == nil {
					t.Fatalf("launch report: %+v", launch.Check)
				}
				out := e.MustRun("pack", "launch", base.Name, "--dry-run")
				if tc.level != "" && (!strings.Contains(out, tc.level) || strings.Index(out, "game") > strings.Index(out, "links")) {
					t.Fatalf("report must precede launch actions: %s", out)
				}
				if tc.name == "different-build" {
					if _, err := e.App.LaunchPack(base.Name, false); err != nil {
						t.Fatalf("warning blocked actual launch: %v", err)
					}
				}
			}
		})
	}
	// Either side lacking a build is informational; malformed versions stay unknown.
	for _, version := range []string{"1.6", ""} {
		g := *game
		g.GameVersion = version
		if c := e.App.CheckPack(base, &g, nil); c.Blocked() || checkItem(c, "game", finding.Info) == nil {
			t.Fatalf("unknown game build: %+v", c)
		}
	}
	problem := app.PackProblem{PackageID: "test.mod", Name: "Changed Mod", Folder: "changed", Why: "content differs"}
	if c := e.App.CheckPack(base, game, []app.PackProblem{problem}); !c.Blocked() || checkItem(c, "mods", finding.Error) == nil {
		t.Fatalf("copy problem: %+v", c)
	}
	// The comparison uses the requested environment rather than the current game.
	other := &env.Environment{ID: "other", GameVersion: game.GameVersion}
	if c := e.App.CheckPack(base, other, nil); !c.Blocked() {
		t.Fatalf("other game has no installed Core: %+v", c)
	}
}

func TestCheckPackInstalledDLCAndList(t *testing.T) {
	e := packEnv(t)
	e.WriteMod(filepath.Join(e.Game, "Data", "Biotech"), testkit.Sample("ludeon.rimworld.biotech", "Biotech"))
	scanNow(t, e)
	game, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	p := app.Pack{Format: 2, Name: "list", Game: game.GameVersion, Light: true, Entries: []app.PackEntry{{PackageID: "ludeon.rimworld", Name: "Core", Official: true}, {PackageID: "test.missing", Name: "Missing Mod"}}}
	c := e.App.CheckPack(&p, game, nil)
	item := checkItem(c, "dlc", finding.Info)
	if c.Blocked() || item == nil || strings.Join(item.Names, ",") != "Biotech" {
		t.Fatalf("unused DLC: %+v", c)
	}
	p.Entries = append(p.Entries, app.PackEntry{PackageID: "LUDEON.RIMWORLD.BIOTECH", Name: "Biotech", Official: true})
	if c := e.App.CheckPack(&p, game, nil); c.Blocked() || checkItem(c, "dlc", finding.Info) != nil {
		t.Fatalf("installed DLC: %+v", c)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(e.Root, "list.archolist.json")
	e.WriteFile(file, string(b))
	r, err := e.App.ImportPack(file, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Check == nil || r.Check.Blocked() || checkItem(r.Check, "mods", finding.Warning) == nil || len(r.Missing) != 1 {
		t.Fatalf("list comparison: %+v", r)
	}
	if out := e.MustRun("pack", "import", file); !strings.Contains(out, "warning: mods not installed: Missing Mod") {
		t.Fatal(out)
	}
}
