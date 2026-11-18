package app

import (
	"os"
	"testing"
	"time"
)

// Ubuntu 26.04's /bin/sleep is a multicall coreutils binary that picks its tool
// from the name it runs under, so a copy under another name exits at once. The
// Linux process tests copy this test binary instead, which sleeps under any name.
const sleeperEnv = "ARCHO_TEST_SLEEPER"

func init() {
	if os.Getenv(sleeperEnv) != "" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

// sleeperProgram returns an executable that sleeps for 30 seconds when run
// with sleeperEnvVar in its environment.
func sleeperProgram(t *testing.T) []byte {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

var sleeperEnvVar = sleeperEnv + "=1"
