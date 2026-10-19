package app_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rimarcho/rimarcho/internal/testkit"
)

// U10 / 04 §6, and the inspector's test_copy_local_creates_clean_independent_git_baseline.
func TestCopyLocal(t *testing.T) {
	e := testkit.New(t)
	ws := e.AddWorkshop("123456", testkit.Sample("example.mod", "Example Mod"))
	e.WriteFile(filepath.Join(ws, ".gitignore"), "ignored.txt\n")
	e.WriteFile(filepath.Join(ws, "ignored.txt"), "must be in baseline\n")
	e.WriteFile(filepath.Join(ws, ".git", "upstream-marker"), "do not copy\n")
	outside := filepath.Join(e.Root, "elsewhere")
	e.WriteFile(filepath.Join(outside, "secret.txt"), "outside the mod\n")
	e.Junction(filepath.Join(ws, "Textures", "External"), outside)
	e.Junction(filepath.Join(ws, "DefsAlias"), filepath.Join(ws, "Defs"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld", "example.mod")
	e.AddEnv(false)
	e.MustRun("scan")
	before := e.ReadModsConfig()
	orig := ref(t, e, "Example Mod")
	out := e.MustRun("copy-local", orig, "--git")
	dst := filepath.Join(e.Game, "Mods", "123456_local")
	for _, want := range []string{"copied to", "git: repository with a baseline commit", "skipped: 1 links", "Textures/External", "loadout: unchanged", "archo swap " + orig + " #"} {
		if !strings.Contains(e.Scrub(out), want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, ".git", "upstream-marker")); err == nil {
		t.Fatal("upstream .git copied")
	}
	if _, err := os.Stat(filepath.Join(dst, "Textures", "External")); err == nil {
		t.Fatal("followed a link outside the mod folder")
	}
	if _, err := os.Stat(filepath.Join(dst, "DefsAlias", "ThingDefs", "examplemod.xml")); err != nil {
		t.Fatal("a link inside the mod folder is copied as content")
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dst
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, b)
		}
		return strings.TrimSpace(string(b))
	}
	if git("ls-files", "ignored.txt") != "ignored.txt" || git("ls-files", ".rimarcho/origin.json") == "" || git("status", "--short") != "" {
		t.Fatal("baseline must hold every file, ignored ones and the origin marker included, and leave a clean tree")
	}
	var o map[string]string
	b, _ := os.ReadFile(filepath.Join(dst, ".rimarcho", "origin.json"))
	if err := json.Unmarshal(b, &o); err != nil || !strings.HasPrefix(o["origin_instance"], strings.TrimPrefix(orig, "#")) || o["workshop_id"] != "123456" || o["origin_lineage"] == "" || o["origin_snapshot"] == "" || o["copied_at"] == "" {
		t.Fatalf("origin.json: %s", b)
	}
	if e.ReadModsConfig() != before {
		t.Fatal("ModsConfig.xml changed")
	}
	if st := e.MustRun("status"); !strings.Contains(st, "same as") && !strings.Contains(st, "0 changes") && strings.Contains(st, "unsaved") {
		t.Fatalf("workspace changed:\n%s", st)
	}
	ix, _ := e.App.Index()
	var lin []string
	for _, m := range ix.Mods {
		if m.Name == "Example Mod" {
			lin = append(lin, m.LineageID)
		}
	}
	if len(lin) != 2 || lin[0] != lin[1] {
		t.Fatalf("copy joins the origin's lineage: %v", lin)
	}
	if _, code := e.Run("copy-local", orig, "--git"); code != 1 {
		t.Fatal("an existing folder must not be overwritten")
	}
	if _, code := e.Run("copy-local", orig, "--name", `..\up`); code != 1 {
		t.Fatal("--name must be a plain folder name")
	}
}
