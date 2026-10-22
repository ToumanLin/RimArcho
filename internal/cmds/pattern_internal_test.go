package cmds

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/logs"
	"github.com/ToumanLin/RimArcho/internal/patterns"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// A mod-specific pattern is shown only when the cluster's own attribution names that mod as
// source; the generic kind is shown either way (01-范式库接入草案.md §3).
func TestClusterPatterns(t *testing.T) {
	p := patterns.Load().Get("m:krkr.rocketman:e84d5a3b")
	if p == nil {
		t.Fatal("fixture pattern missing")
	}
	ix := &app.Index{ByID: map[string]*store.Mod{
		"i1": {ID: "i1", PackageID: "krkr.rocketman_steam"},
		"i2": {ID: "i2", PackageID: "other.mod"},
	}}
	render1 := func(attr ...*app.Attribution) string {
		lc := &app.LogCluster{Cluster: &logs.Cluster{ID: "E7", Class: p.Class, Template: p.Template}, Attr: attr}
		d, root := render.New("x")
		addClusterPatterns(root, ix, lc)
		return d.String() + patternTag(ix, lc)
	}
	out := render1(&app.Attribution{Instance: "i1", Role: app.RoleSource})
	for _, want := range []string{"pattern: pattern:g:asm.version-mismatch", "evidence: community:", "known-error: pattern:m:krkr.rocketman:e84d5a3b", "may have fixed it", "[pattern:g:asm.version-mismatch]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, attr := range []*app.Attribution{{Instance: "i1", Role: app.RolePatcher}, {Instance: "i2", Role: app.RoleSource}} {
		if out := render1(attr); strings.Contains(out, "known-error") || !strings.Contains(out, "pattern:g:asm.version-mismatch") {
			t.Errorf("%s as %s:\n%s", attr.Instance, attr.Role, out)
		}
	}
	// Fallback kinds (a bare NullReferenceException) add nothing.
	lc := &app.LogCluster{Cluster: &logs.Cluster{ID: "E1", Class: "exception", Template: "exception|System.NullReferenceException: Object reference not set to an instance of an object"}}
	d, root := render.New("x")
	addClusterPatterns(root, ix, lc)
	if got := d.String() + patternTag(ix, lc); got != "x\n" {
		t.Errorf("fallback rendered: %q", got)
	}
	if got := clusterRoute(patterns.Load().Get("g:xref.def"), "E3"); got != "get_def <name> → get_patches failed=true → list_problems code=dep" {
		t.Errorf("route: %s", got)
	}
}
