package cmds

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestMCPHintsCoverCommands is the static check of 14 §3: every archo command the code
// prints (in a string literal of cmds, app or view) is one MCPHints turns into an MCP tool
// call, or is in cliOnlyHints with the reason it stays a CLI command. A new CLI-style hint
// on the MCP surface fails here until it gets a hintSpecs row or a cliOnlyHints entry.
func TestMCPHintsCoverCommands(t *testing.T) {
	lit := regexp.MustCompile(`"(?:[^"\\]|\\.)*"|` + "`[^`]*`")
	cmd := regexp.MustCompile(`\barcho ((?:[a-z][a-z-]*)(?: [a-z][a-z-]*)?)`)
	missing := map[string][]string{}
	for _, dir := range []string{".", "../app", "../view"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			base := filepath.Base(f)
			// The guide describes both surfaces on purpose and is not rewritten (14 §3).
			if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "guide") || base == "mcphints.go" {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range lit.FindAllString(string(b), -1) {
				for _, m := range cmd.FindAllStringSubmatch(l, -1) {
					words := strings.Fields(m[1])
					c, _ := lookup(words)
					if c == nil {
						continue // prose: "archo can run", "archo before the"
					}
					if sp := hintSpecFor(strings.Fields(c.Name)); sp != nil && sp.tool != "" {
						continue
					}
					if _, ok := cliOnlyHints[c.Name]; ok {
						continue
					}
					if !slices.Contains(missing[c.Name], base) {
						missing[c.Name] = append(missing[c.Name], base)
					}
				}
			}
		}
	}
	for name, files := range missing {
		t.Errorf("archo %s (in %s) has no MCP tool in hintSpecs and is not in cliOnlyHints", name, strings.Join(files, ", "))
	}
	for name := range cliOnlyHints {
		if sp := hintSpecFor(strings.Fields(name)); sp != nil && sp.tool != "" {
			t.Errorf("archo %s is in cliOnlyHints but has a tool (%s)", name, sp.tool)
		}
	}
}
