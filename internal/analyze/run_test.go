package analyze

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Run's less travelled paths: parse errors in each layer, cached and indexed inactive Mods,
// a patch file that is not a <Patch>, progress every 500 Def files and 1000 operations,
// AfterSimulation, the default Harmony skip list and unreadable DLLs.
func TestRunEdgePaths(t *testing.T) {
	root := t.TempDir()
	p := &Mod{Instance: "p", Pkg: "test.p", Name: "P", Pos: 1, Dir: filepath.Join(root, "p"), SnapHash: []byte("p")}
	q := &Mod{Instance: "q", Pkg: "test.q", Name: "Q", Dir: filepath.Join(root, "q"), SnapHash: []byte("q")}
	r := &Mod{Instance: "r", Pkg: "test.r", Name: "R", Dir: filepath.Join(root, "r"), SnapHash: []byte("r")}
	for i := range 500 {
		write(t, filepath.Join(p.Dir, "Defs", fmt.Sprintf("D%03d.xml", i)), defs(fmt.Sprintf(`<ThingDef><defName>T%d</defName></ThingDef>`, i)))
	}
	write(t, filepath.Join(p.Dir, "Defs", "Bad.xml"), "<Defs><ThingDef>")
	var ops []string
	for i := range 1000 {
		ops = append(ops, fmt.Sprintf(`<Operation Class="PatchOperationAdd"><xpath>Defs/ThingDef[defName="T%d"]</xpath><value><x/></value></Operation>`, i%500))
	}
	write(t, filepath.Join(p.Dir, "Patches", "Many.xml"), patch(ops...))
	write(t, filepath.Join(p.Dir, "Patches", "Bad.xml"), "<Patch><Operation>")
	write(t, filepath.Join(p.Dir, "Patches", "NotPatch.xml"), defs(`<Operation Class="PatchOperationRemove"><xpath>Defs</xpath></Operation>`))
	write(t, filepath.Join(p.Dir, "Assemblies", "Broken.dll"), "not a PE file")
	write(t, filepath.Join(p.Dir, "Assemblies", "0Harmony.dll"), "not a PE file")
	write(t, filepath.Join(q.Dir, "Defs", "Q.xml"), defs(`<ThingDef><defName>Q</defName></ThingDef>`))
	write(t, filepath.Join(r.Dir, "Defs", "R.xml"), defs(`<ThingDef><defName>R</defName></ThingDef>`))
	write(t, filepath.Join(r.Dir, "Defs", "Bad.xml"), "<Defs><ThingDef>")

	var progress []string
	after := 0
	in := Input{Mods: []*Mod{q, p, r}, MM: "1.6",
		Cached: func(instance, key string) ([]DefFact, bool) {
			if instance == "q" {
				return []DefFact{{Type: "ThingDef", DefName: "CachedQ", File: "Defs/Q.xml", Line: 1}}, true
			}
			return nil, false
		},
		Progress:        func(s string) { progress = append(progress, s) },
		AfterSimulation: func() { after++ },
	}
	out, err := Run(in)
	if err != nil {
		t.Fatal(err)
	}
	type asm struct {
		Instance, File string
		Skipped, Err   bool
	}
	st := out.Stats
	st.DefsMs, st.PatchMs, st.TotalMs, st.HarmonyMs, st.PeakHeapMB, st.LiveHeapMB = 0, 0, 0, 0, 0, 0
	st.SlowestOp, st.SlowestMs = "", 0
	view := struct {
		Progress []string
		After    int
		Stats    Stats
		Content  map[string]*ContentState
		NewDefs  map[string]int
		Asms     []asm
		Findings []string
		Patches  int
	}{After: after, Stats: st, Content: out.Content, NewDefs: map[string]int{}, Patches: len(out.Patches)}
	done := regexp.MustCompile(`, [0-9.]+[µm]?s$`)
	for _, s := range progress {
		view.Progress = append(view.Progress, done.ReplaceAllString(s, ", <time>"))
	}
	for _, cs := range out.Content {
		if cs.Key == "" {
			t.Error("a content state without a key")
		}
		cs.Key = "(varies with file times)"
	}
	for k, v := range out.NewDefs {
		view.NewDefs[k] = len(v)
	}
	for _, a := range out.Assemblies {
		view.Asms = append(view.Asms, asm{a.Instance, filepath.ToSlash(a.File), a.Skipped, a.Err != ""})
	}
	for _, f := range out.Findings {
		view.Findings = append(view.Findings, f.Code+" "+f.Key)
	}
	b, err := json.MarshalIndent(view, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ReplaceAll(strings.ReplaceAll(string(b), `\\`, "/"), filepath.ToSlash(root), "<root>") + "\n"
	want, err := os.ReadFile("testdata/run_edge_paths.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("Run output changed:\n%s", got)
	}
}
