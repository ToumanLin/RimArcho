package ids

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestNewAndShort(t *testing.T) {
	r := rand.NewChaCha8([32]byte{1})
	var all []string
	for i := 0; i < 2000; i++ {
		id := New(r)
		if len(id) != 26 || !Valid(id) || strings.ToLower(id) != id {
			t.Fatalf("bad id %q", id)
		}
		all = append(all, id)
	}
	s := NewShortener(all)
	seen := map[string]bool{}
	for _, id := range all {
		p := s.Short(id)
		if len(p) < MinPrefix || !strings.HasPrefix(id, p) || seen[p] {
			t.Fatalf("short %q of %q", p, id)
		}
		seen[p] = true
		if got := s.Lookup(p); len(got) != 1 || got[0] != id {
			t.Fatalf("lookup %q = %v", p, got)
		}
	}
	if Valid("ab1") || Valid("AbCD") || Valid("ab#d") {
		t.Fatal("Valid accepted a bad prefix")
	}
}
