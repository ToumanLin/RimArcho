package rules

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/order"
)

func TestLoad(t *testing.T) {
	p := filepath.Join(t.TempDir(), "communityRules.json")
	os.WriteFile(p, []byte(`{"timestamp": 5, "rules": {"A.Mod": {"loadAfter": {"b.mod": {"name": ["B"]}}, "loadBefore": {"C.mod": {}}}}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	after, before := c.Rules("a.mod")
	if !slices.Equal(after, []string{"b.mod"}) || !slices.Equal(before, []string{"c.mod"}) || c.Count() != 1 {
		t.Fatalf("%v %v", after, before)
	}
	if c, err := Load(filepath.Join(t.TempDir(), "none.json")); err != nil || c.Count() != 0 {
		t.Fatal("missing file must be empty")
	}
}

func TestLoadPositions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "communityRules.json")
	os.WriteFile(p, []byte(`{"rules": {
		"Krkr.RocketMan": {"loadBottom": {"comment": "last", "value": true}},
		"imranfish.xmlextensions": {"loadTop": {"value": "true"}},
		"off.mod": {"loadTop": {"value": false}},
		"both.mod": {"loadTop": {"value": true}, "loadBottom": {"value": true}}}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for pkg, want := range map[string]order.Tier{"krkr.rocketman": order.TierBottom, "imranfish.xmlextensions": order.TierTop,
		"off.mod": order.TierNormal, "both.mod": order.TierNormal, "other.mod": order.TierNormal} {
		if got := c.Position(pkg); got != want {
			t.Errorf("%s: %v, want %v", pkg, got, want)
		}
	}
	if c.Count() != 2 {
		t.Fatalf("count %d", c.Count())
	}
}
