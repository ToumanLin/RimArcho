package tui

import (
	"slices"
	"testing"
)

// A log cluster reference on the technical page is followed to the cluster (06 §2.10).
func TestLineRefLogCluster(t *testing.T) {
	t.Parallel()
	got := lineRef("    error log.exception  log:E3 x512: boom  (p:abcdef)", nil)
	if !slices.Equal(got, []string{"log", "--cluster", "E3"}) {
		t.Fatalf("lineRef = %v", got)
	}
}
