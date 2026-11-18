package render

import "testing"

// Words ending in s, x, z, ch or sh take "es" (VM round 2026-11-10: a scan said "2 aliass").
func TestPlural(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		n          int
		word, want string
	}{
		{1, "alias", "1 alias"}, {2, "alias", "2 aliases"}, {7, "class", "7 classes"}, {2, "patch", "2 patches"},
		{3, "box", "3 boxes"}, {2, "mod", "2 mods"}, {2, "copy", "2 copies"}, {2, "key", "2 keys"}, {0, "subscription", "0 subscriptions"},
	} {
		if got := Plural(c.n, c.word); got != c.want {
			t.Errorf("Plural(%d, %q) = %q, want %q", c.n, c.word, got, c.want)
		}
	}
}
