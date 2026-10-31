package gui

import (
	"os/exec"
	"testing"
)

func TestWindowChrome(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for window chrome behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/window-chrome.mjs").CombinedOutput(); err != nil {
		t.Fatalf("window chrome: %v\n%s", err, out)
	}
}

func TestCommandPanel(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for command panel behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/command-panel.mjs").CombinedOutput(); err != nil {
		t.Fatalf("command panel: %v\n%s", err, out)
	}
}
