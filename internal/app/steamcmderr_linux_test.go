//go:build linux

package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSteamCMDMissingLibraries(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "steamcmd")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho 'error while loading shared libraries: libstdc++.so.6' >&2\nexit 127\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, run := range []func() error{
		func() error { _, err := runSteamCMD(exe, nil); return err },
		func() error { _, err := runBranchSteamCMD(context.Background(), exe, nil); return err },
		func() error {
			p, err := startWorkshopCMD(exe, nil, true)
			if err != nil {
				return err
			}
			return p.Wait()
		},
	} {
		err := run()
		if err == nil {
			t.Fatal("missing loader succeeded")
		}
		for _, name := range []string{"lib32gcc-s1", "glibc.i686", "libstdc++.i686", "lib32-gcc-libs"} {
			if !strings.Contains(err.Error(), name) {
				t.Fatal(err)
			}
		}
	}
}
