package gui

import (
	"os/exec"
	"testing"
)

func TestEditorFilterBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for editor filter tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/editorfilter.mjs", "testdata/detail-follow.mjs", "testdata/status-motion.mjs").CombinedOutput(); err != nil {
		t.Fatalf("editor filter: %v\n%s", err, out)
	}
}
