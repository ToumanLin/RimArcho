package gui

import (
	"io/fs"
	"strings"
	"testing"
)

// Every adjustable card gap uses the keyboard and persistence implementation (16 §3.1).
func TestSeparators(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"main", "filterbar", "library", "screens", "tasks", "planmanage", "shell"} {
		b, err := fs.ReadFile(assets, "assets/js/"+name+".js")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `from "./separator.js"`) || !strings.Contains(string(b), "wireSeparator(") {
			t.Errorf("%s does not use the shared separator", name)
		}
	}
}
