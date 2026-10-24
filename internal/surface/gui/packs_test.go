package gui

import (
	"os/exec"
	"testing"
)

func TestPackPageComparison(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for the pack page behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/packs.mjs").CombinedOutput(); err != nil {
		t.Fatalf("pack page: %v\n%s", err, out)
	}
}
