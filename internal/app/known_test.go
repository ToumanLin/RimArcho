package app_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/finding"
	"github.com/ToumanLin/RimArcho/internal/known"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Findings from the log corpus dataset (internal/known): undeclared prerequisites and
// known-bad combinations, computed from the enabled list.
func TestKnownFindings(t *testing.T) {
	defer known.SetForTesting(&known.Data{
		Prereqs: []*known.Prereq{
			{Mod: "tester.user", Needs: "tester.lib", Logs: 40, Players: 30, Without: 50, Items: []string{"type Lib.Comp"}, Class: "xml-load", Example: "xml-load|Could not find a type named Lib.Comp"},
			{Mod: "tester.user", Needs: "tester.absent", Logs: 10, Players: 8, Without: 40, Items: []string{"def SoundDef Boom"}, Class: "xref-missing", Cosmetic: true,
				Fix: &known.Fix{Pairs: 4, Gone: 2, Same: 1, Changes: map[string]int{"enabled tester.absent": 2}}},
			{Mod: "tester.user", Needs: "tester.declared", Logs: 10, Players: 8, Without: 10}, // declared by the installed version: dep.* covers it
			{Mod: "tester.user", Needs: "tester.other", Logs: 10, Players: 8, Without: 10},    // enabled: nothing to say
		},
		Combos: []*known.Combo{
			{Mods: []string{"tester.user", "tester.other"}, Class: "patch-failed", Templates: []string{"patch-failed|[User] Patch operation failed"}, Logs: 30, Players: 20, With: 40, Without: 100, WithoutErr: 2},
			{Mods: []string{"tester.user", "tester.lib"}, Class: "patch-failed", Templates: []string{"patch-failed|x"}, Logs: 30, Players: 20, With: 40, Without: 100}, // lib not enabled
			{Mods: []string{"tester.other"}, Version: "2.0", Class: "exception", Templates: []string{"exception|Error in static constructor"}, Logs: 9, Players: 6, With: 10, Without: 80},
			{Mods: []string{"tester.other"}, Version: "1.0", Class: "exception", Templates: []string{"exception|old"}, Logs: 9, Players: 6, With: 10, Without: 80},             // other version
			{Mods: []string{"tester.lib"}, Version: "3.1", Class: "exception", Templates: []string{"exception|from the manifest"}, Logs: 9, Players: 6, With: 10, Without: 80}, // Manifest.xml version
			{Mods: []string{"tester.user"}, Class: "xref-missing", Templates: []string{"xref-missing|Could not resolve cross-reference: No Verse.SoundDef named … found"}, Logs: 90, Players: 60, With: 100},
		},
	})()
	e := setup(t, false)
	lib := testkit.Sample("tester.lib", "Lib")
	user := testkit.Sample("tester.user", "User")
	user.Deps = []string{"tester.declared"}
	e.WriteMod(filepath.Join(e.Game, "Mods", "Lib"), lib)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Lib", "About", "Manifest.xml"), "<Manifest><identifier>Lib</identifier><version>3.1</version></Manifest>")
	e.WriteMod(filepath.Join(e.Game, "Mods", "User"), user)
	e.WriteFile(filepath.Join(e.Game, "Mods", "Other", "About", "About.xml"), `<ModMetaData><packageId>tester.other</packageId><name>Other</name>
<supportedVersions><li>1.6</li></supportedVersions><modVersion>2.0</modVersion></ModMetaData>`)
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.other", "tester.user")
	scanNow(t, e)
	ps := problems(t, e)
	all := append(append([]*appProblem{}, ps.List...), ps.Ignored...)
	userTok, libTok, otherTok := finding.Inst(instOf(t, e, "pkg:tester.user")), finding.Inst(instOf(t, e, "pkg:tester.lib")), finding.Inst(instOf(t, e, "pkg:tester.other"))

	obs := map[string]*appProblem{}
	for _, p := range byCode(all, "dep.observed-missing") {
		obs[p.Key] = p
	}
	if len(obs) != 2 || obs["tester.lib"] == nil || obs["tester.absent"] == nil {
		t.Fatalf("dep.observed-missing: %v", keys(obs))
	}
	inactive := obs["tester.lib"]
	if inactive.Severity != finding.Warning || inactive.Subjects[1] != libTok || inactive.Suggestions[0] != "archo enable "+libTok ||
		!strings.Contains(inactive.Claim, "installed but not enabled") || !strings.Contains(inactive.Evidence[0].Text, "40 of 50") ||
		inactive.Evidence[0].Source != finding.Community {
		t.Fatalf("installed provider: %+v", inactive.Finding)
	}
	absent := obs["tester.absent"]
	if absent.Severity != finding.Info || absent.Subjects[1] != "pkg:tester.absent" || !strings.Contains(absent.Claim, "sounds or textures") {
		t.Fatalf("absent cosmetic provider: %+v", absent.Finding)
	}
	fixed := false
	for _, ev := range absent.Evidence {
		fixed = fixed || strings.Contains(ev.Text, "gone in 2 of 4, 2 of them after enabled pkg:tester.absent")
	}
	if !fixed {
		t.Fatalf("session evidence missing: %+v", absent.Evidence)
	}

	combos := byCode(all, "combo.known-bad")
	if len(combos) != 2 {
		t.Fatalf("combo.known-bad: %+v", combos)
	}
	for _, c := range combos {
		switch c.Subjects[0] {
		case userTok: // the pair
			if len(c.Subjects) != 2 || c.Subjects[1] != otherTok || c.Severity != finding.Warning || c.Suggestions[0] != "archo disable "+otherTok ||
				!strings.Contains(c.Claim, "75% of shared logs with both") || !strings.Contains(c.Claim, "2% without") {
				t.Fatalf("pair: %+v", c.Finding)
			}
		case otherTok: // version 2.0 matches About.xml modVersion
			if !strings.Contains(c.Claim, "version 2.0") || c.Severity != finding.Warning {
				t.Fatalf("version: %+v", c.Finding)
			}
		default:
			t.Fatalf("unexpected subject: %+v", c.Finding)
		}
	}
	ke := byCode(all, "mod.known-error")
	if len(ke) != 1 || ke[0].Severity != finding.Info || ke[0].Subjects[0] != userTok || !strings.Contains(ke[0].Claim, "90%") {
		t.Fatalf("mod.known-error: %+v", ke)
	}

	// Enabling the provider clears its finding; the pair with lib and lib's version (from
	// About/Manifest.xml) now apply.
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "tester.lib", "tester.other", "tester.user")
	ps = problems(t, e)
	for _, p := range byCode(ps.List, "dep.observed-missing") {
		if p.Key == "tester.lib" {
			t.Fatalf("still reported after enabling: %+v", p.Finding)
		}
	}
	if n := len(byCode(ps.List, "combo.known-bad")); n != 4 {
		t.Fatalf("combo.known-bad after enabling lib: %d", n)
	}
}

type appProblem = app.Problem

func keys(m map[string]*appProblem) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
