package gui

import (
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
)

// 贴图优化 in the window: the page, a run in the background (the call returns at once, the
// pages answer, a second run is refused), its progress and its end as "textures" events, in
// the interface's words.
func TestTexturesBackground(t *testing.T) {
	e, api, events := fixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	p, err := api.Textures("plan", 50)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mods == 0 || p.ToolOK || p.Offer == "" || p.Scale != 50 {
		t.Fatalf("page: mods %d tool %v offer %q scale %d", p.Mods, p.ToolOK, p.Offer, p.Scale)
	}
	release := make(chan struct{})
	var got []string
	api.TexRunner = func(args []string, progress func(string)) ([]string, error) {
		got = args
		progress(cmds.TextureProgressLine(app.TextureProgress{Files: 1, FilesTotal: 4, Mods: 0, ModsTotal: 1, Mod: "Alpha"}))
		<-release
		return []string{"converted 4 textures to DDS (1.0 MB written)", "failed: 1 (they keep their PNG)"}, nil
	}
	start := time.Now()
	if _, err := api.TexturesRun("plan", 50, nil); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("TexturesRun waited for the run")
	}
	if _, err := api.TexturesRun("plan", 100, nil); err == nil {
		t.Fatal("a second run started")
	}
	if _, err := api.TexturesClean("plan"); err == nil {
		t.Fatal("clean while a run goes on")
	}
	wait := func(want string) string {
		deadline := time.After(10 * time.Second)
		for {
			select {
			case ev := <-events:
				if strings.HasPrefix(ev, "textures ") && strings.Contains(ev, want) {
					return ev
				}
			case <-deadline:
				t.Fatalf("no textures event with %q", want)
			}
		}
	}
	wait(`"frac":0.25`)
	if r, _ := api.Textures("plan", 50); r.Running == nil {
		t.Fatal("the page does not show the run")
	}
	close(release)
	ev := wait(`"done":true`)
	if !strings.Contains(ev, "converted 4 textures to DDS (1.0 MB written) · 1 failed") {
		t.Fatalf("end: %s", ev)
	}
	if strings.Join(got, " ") != "textures optimize --plan --scale 50" {
		t.Fatalf("args %v", got)
	}
	// Mod 库存's selection
	release = make(chan struct{})
	close(release)
	api.TexturesRun("", 100, []string{"#abc"})
	wait(`"done":true`)
	if strings.Join(got, " ") != "textures optimize #abc" {
		t.Fatalf("refs: %v", got)
	}
}
