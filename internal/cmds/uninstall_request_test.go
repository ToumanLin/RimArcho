package cmds

import "testing"

func TestParseUninstallRequest(t *testing.T) {
	for _, tc := range []struct {
		argv     []string
		run, bad bool
	}{
		{[]string{"uninstall"}, false, false},
		{[]string{"uninstall", "--dry-run"}, false, false},
		{[]string{"uninstall", "--yes", "--keep-locked"}, true, false},
		{[]string{"uninstall", "--saves-to", "--yes"}, false, false},
		{[]string{"uninstall", "--yes", "--dry-run"}, false, true},
		{[]string{"uninstall", "--yes=true"}, false, true},
	} {
		_, run, err := ParseUninstallRequest(tc.argv)
		if run != tc.run || (err != nil) != tc.bad {
			t.Errorf("%v: run=%v err=%v", tc.argv, run, err)
		}
	}
}
