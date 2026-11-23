//go:build !windows

package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Outside Windows the data and program go while uninstall runs (02 §1): the report says
// they are deleted, marks them done, and names no log of leftovers.
func TestUninstallDeletesAtOnce(t *testing.T) {
	f := uninstallFixture(t)
	f.App.AfterExitFunc = nil
	plan := f.MustRun("uninstall")
	if strings.Contains(plan, "once RimArcho has closed") || strings.Contains(plan, "after exit") {
		t.Fatalf("the plan defers what is deleted at once:\n%s", plan)
	}
	out := f.MustRun("uninstall", "--yes")
	if !strings.HasPrefix(out, "uninstalled: RimArcho's folders and program are deleted\n") ||
		strings.Contains(out, "once this command ends") || strings.Contains(out, "RimArcho-uninstall.log") || strings.Contains(out, "later ") {
		t.Fatalf("report:\n%s", out)
	}
	if !strings.Contains(out, "done   delete      "+f.Home) || !strings.Contains(out, "done   delete      "+f.program) {
		t.Fatalf("data and program are not reported done:\n%s", out)
	}
	for _, p := range []string{f.Home, filepath.Join(f.program, "archo.exe")} {
		if exists(p) {
			t.Errorf("%s is still there", p)
		}
	}
}

// What could not be deleted is a failure on its item, not a promise of later.
func TestUninstallAtOnceReportsLeftovers(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-only folder")
	}
	f := uninstallFixture(t)
	f.App.AfterExitFunc = nil
	if err := os.Chmod(f.program, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(f.program, 0o755) })
	out, _ := f.Run("uninstall", "--yes")
	if !strings.Contains(out, "FAILED delete      "+f.program) || !strings.Contains(out, "could not delete "+f.program+"/") {
		t.Fatalf("report:\n%s", out)
	}
	if !exists(filepath.Join(f.program, "archo.exe")) {
		t.Fatal("the read-only program folder lost its file")
	}
}
