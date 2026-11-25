package mcp_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestMaintenanceLogCleanupConfirmation(t *testing.T) {
	e := testkit.New(t)
	e.AddEnv(false)
	path := filepath.Join(e.UserData, "Player-prev.log")
	e.WriteFile(path, "RimWorld 1.6.4633 rev1261\n")
	if err := os.Truncate(path, app.LogSizeLimit+1); err != nil {
		t.Fatal(err)
	}
	cs := connect(t, e)
	args := map[string]any{"action": "delete_log", "prev": true}
	preview, isErr := call(t, cs, "maintenance", args)
	if isErr || !strings.Contains(preview, "permanently delete this file") {
		t.Fatalf("preview: %s", preview)
	}
	args["confirm"] = true
	if p, isErr := call(t, cs, "maintenance", args); !isErr || !strings.Contains(p, "--token") {
		t.Fatalf("confirmation with no token: %s", p)
	}
	for _, line := range strings.Split(preview, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "token: "); ok {
			args["token"] = value
			break
		}
	}
	if args["token"] == nil {
		t.Fatalf("missing token in preview: %s", preview)
	}
	args["dry_run"] = true
	if p, isErr := call(t, cs, "maintenance", args); isErr || !strings.Contains(p, "preview") {
		t.Fatalf("forced dry run: %s", p)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	delete(args, "dry_run")
	if p, isErr := call(t, cs, "maintenance", args); isErr || !strings.Contains(p, "game log deleted") {
		t.Fatalf("confirmed deletion: %s", p)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file after confirmed deletion: %v", err)
	}
}
