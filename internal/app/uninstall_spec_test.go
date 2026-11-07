package app_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// 02 §2: every row of the write list (02 §1) says what uninstall does with it — a step in
// backticks, or "none:" and why not — every step it names exists, and every step is in 02 §1.
func TestUninstallCoversWriteList(t *testing.T) {
	t.Parallel()
	_, file, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "docs", "specs", "02-file-writes-and-uninstall.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(string(b), "\r\n", "\n")
	start := strings.Index(doc, "\n## 1. ")
	end := strings.Index(doc, "\n## 2. ")
	if start < 0 || end < start {
		t.Fatal("02 §1 not found")
	}
	sec := doc[start:end]
	known := map[string]bool{}
	for _, k := range app.UninstallSteps {
		known[k] = true
	}
	tick := regexp.MustCompile("`([a-z]+)`")
	rows, header := 0, false
	for _, l := range strings.Split(sec, "\n") {
		if !strings.HasPrefix(l, "|") {
			if header && rows > 0 {
				break // the table has ended
			}
			continue
		}
		cells := strings.Split(strings.Trim(l, "|"), " | ")
		if !header {
			header = true
			if last := strings.TrimSpace(cells[len(cells)-1]); !strings.HasPrefix(last, "On uninstall") {
				t.Fatalf("the write list has no uninstall column: %q", last)
			}
			continue
		}
		if strings.HasPrefix(l, "| ---") {
			continue
		}
		rows++
		last := strings.TrimSpace(cells[len(cells)-1])
		named := false
		for _, m := range tick.FindAllStringSubmatch(last, -1) {
			if known[m[1]] {
				named = true
			}
		}
		if strings.Index(last, "`") == 0 && !named {
			t.Errorf("row %q names no uninstall step: %q", cells[0], last)
		}
		if !named && !strings.HasPrefix(last, "None:") {
			t.Errorf("row %q: no uninstall step and no reason (none: …): %q", cells[0], last)
		}
	}
	if rows < 15 {
		t.Fatalf("only %d rows read from the write list", rows)
	}
	for _, k := range app.UninstallSteps {
		if !strings.Contains(sec, "`"+k+"`") {
			t.Errorf("step %s is not in 02 §1", k)
		}
	}
}
