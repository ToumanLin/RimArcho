package gui

import (
	"io/fs"
	"os/exec"
	"strings"
	"testing"
)

// Every adjustable card gap uses the keyboard and persistence implementation (06 §3.1).
func TestSeparators(t *testing.T) {
	for _, name := range []string{"main", "filterbar", "library", "screens", "tasks", "planmanage", "shell"} {
		b, err := fs.ReadFile(assets, "assets/js/"+name+".js")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `from "./separator.js"`) || !strings.Contains(string(b), "wireSeparator(") {
			t.Errorf("%s does not use the shared separator", name)
		}
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for separator behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/separators.mjs").CombinedOutput(); err != nil {
		t.Fatalf("separators: %v\n%s", err, out)
	}
}
