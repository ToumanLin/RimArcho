package cmds

import (
	"runtime"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func TestSearchNextPreservesScope(t *testing.T) {
	t.Parallel()
	command, _ := lookup([]string{"files", "search"})
	a, err := parse(command, []string{"--mod", "#one", "--mod", "pkg:a.b", "--regex", "--case", "--word", "--def", "--kind", "defs,patches", "--languages", "--lang", "English", "--lang", "Chinese Simplified", "--all-versions", "--game", "without", "--mode", "files", "--limit", "3", "--per-file", "2", "--game-version", "1.5", "--page", "1", "--", "--Odd 'Target' $name"})
	if err != nil {
		t.Fatal(err)
	}
	got := searchNext(&Ctx{App: &app.App{EnvName: "other environment"}}, a, 2)
	for _, part := range []string{"archo --env 'other environment' files search", "--mod '#one'", "--mod pkg:a.b", "--regex --case --word --def", "--kind defs,patches --languages", "--lang English --lang 'Chinese Simplified'", "--all-versions --game without --mode files", "--limit 3 --per-file 2 --game-version 1.5 --page 2 --"} {
		if !strings.Contains(got, part) {
			t.Errorf("missing %q in %s", part, got)
		}
	}
	quoted := `'--Odd '\''Target'\'' $name'`
	if runtime.GOOS == "windows" {
		quoted = `'--Odd ''Target'' $name'`
	}
	if !strings.HasSuffix(got, quoted) || strings.Contains(got, "--page 1") {
		t.Fatal(got)
	}
}
