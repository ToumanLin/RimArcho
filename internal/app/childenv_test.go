package app

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func TestChildEnvMarksHelper(t *testing.T) {
	env := ChildEnv(`C:\home`)
	if !slices.Contains(env, "RIMARCHO_HOME=C:\\home") || !slices.Contains(env, ChildEnvVar+"=1") {
		t.Fatalf("ChildEnv: %v", env[len(env)-2:])
	}
}

// Every helper this package starts with a data directory goes through ChildEnv, so none can
// open a window (17 §3). A new spawn that builds RIMARCHO_HOME by hand fails here.
func TestChildSpawnsUseChildEnv(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	// RIMARCHO_HOME as the last entry: no child marker follows it
	bad := regexp.MustCompile(`append\(os\.Environ\(\),\s*"RIMARCHO_HOME="\+[\w.]+\)`)
	for _, f := range files {
		if filepath.Ext(f) != ".go" || len(f) > 8 && f[len(f)-8:] == "_test.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bad.Match(b) {
			t.Errorf("%s starts a helper without ChildEnv (17 §3)", f)
		}
	}
}
