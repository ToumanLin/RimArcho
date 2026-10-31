package app_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestCommandChildHelper(t *testing.T) {
	mode := os.Getenv("RIMARCHO_COMMAND_CHILD_HELPER")
	if mode == "" {
		return
	}
	if mode == "wait" {
		time.Sleep(30 * time.Second)
		return
	}
	fmt.Print(strings.Join(os.Args[1:], "|"))
	os.Exit(7)
}
func TestChildCommandRawExitAndCancellation(t *testing.T) {
	e := testkit.New(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIMARCHO_COMMAND_CHILD_HELPER", "output")
	argv := []string{"-test.run=^TestCommandChildHelper$", "--", "two words", `$(literal); & #`}
	out, code := app.ChildCommandContext(context.Background(), exe, e.Home, "", argv)
	if code != 7 || out != strings.Join(argv, "|") {
		t.Fatalf("child: %q exit %d", out, code)
	}
	t.Setenv("RIMARCHO_COMMAND_CHILD_HELPER", "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	before := time.Now()
	_, code = app.ChildCommandContext(ctx, exe, e.Home, "", argv)
	if code == 0 || time.Since(before) > 3*time.Second {
		t.Fatalf("child did not stop: exit %d", code)
	}
}
