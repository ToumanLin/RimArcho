package app

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/store"
)

func TestCreditLaunchLockHelper(t *testing.T) {
	home := os.Getenv("RIMARCHO_TEST_LAUNCH_LOCK")
	if home == "" {
		return
	}
	a := &App{Home: home}
	// Both public entry points must refuse before they can read or mutate anything.
	if _, err := a.LaunchWith(false, false); err == nil || !strings.Contains(err.Error(), "another launch") {
		t.Fatalf("launch bypassed process lock: %v", err)
	}
	if _, err := a.ReplaceCreditItem(store.CreditItem{}); err == nil || !strings.Contains(err.Error(), "another launch") {
		t.Fatalf("replacement bypassed process lock: %v", err)
	}
	if _, err := a.LaunchPackWith("fixture", false, false); err == nil || !strings.Contains(err.Error(), "another launch") {
		t.Fatalf("pack launch bypassed process lock: %v", err)
	}
}

func TestCreditLaunchLockAcrossProcesses(t *testing.T) {
	t.Parallel()
	a := &App{Home: t.TempDir()}
	unlock, err := a.lockLaunch()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCreditLaunchLockHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(), "RIMARCHO_TEST_LAUNCH_LOCK="+a.Home)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-process exclusion: %v\n%s", err, out)
	}
}
