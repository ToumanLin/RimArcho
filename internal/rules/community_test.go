package rules

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
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
