package app

import "testing"

// Ignores are keyed by findingKey, so a relative path must pass through unchanged.
func TestFindingKeyKeepsRelativePaths(t *testing.T) {
	if k := "XML error in Defs/ThingDefs/Foo.xml"; findingKey(k) != k {
		t.Errorf("findingKey(%q) = %q", k, findingKey(k))
	}
	if got := findingKey(`load D:\Steam\steamapps\workshop\content\294100\1\Defs\a.xml`); got != "load 1/defs/a.xml" {
		t.Errorf("drive path: %q", got)
	}
}
