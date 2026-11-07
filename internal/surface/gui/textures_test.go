package gui

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func waitTextureMeasure(t *testing.T, api *API, request uint64) *TexturePage {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		p := api.TextureMeasure(request)
		if p == nil {
			t.Fatal("current measurement disappeared")
		}
		if p.Error != "" {
			t.Fatal(p.Error)
		}
		if !p.Loading {
			return p
		}
		select {
		case <-deadline:
			t.Fatal("texture measurement did not finish")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestTextureMeasureReturnsBeforeDiskAndRejectsLateResults(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	// Hold the disk reader: requesting either option must still return a pending page.
	api.texRead.mu.Lock()
	locked := true
	defer func() {
		if locked {
			api.texRead.mu.Unlock()
		}
	}()
	read := func(scope string, scale int) *TexturePage {
		t.Helper()
		ch := make(chan *TexturePage, 1)
		go func() { p, _ := api.Textures(scope, scale); ch <- p }()
		select {
		case p := <-ch:
			if !p.Loading || p.Scope != scope || p.Scale != scale || len(p.Scales) == 0 {
				t.Fatalf("pending page: %+v", p)
			}
			return p
		case <-time.After(time.Second):
			t.Fatal("options waited for disk reads")
			return nil
		}
	}
	old := read("plan", 100)
	next := read("all", 50)
	stampResponds(t, api)
	// Simulate an old completion even if the underlying operation ignores cancellation.
	api.publishTextureMeasure(context.Background(), old.Request, func(m *textureMeasurement) {
		m.page, m.err = old.TexturePage, "late error"
	})
	if api.TextureMeasure(old.Request) != nil {
		t.Fatal("previous options still have a result")
	}
	if p := api.TextureMeasure(next.Request); p.Scope != "all" || p.Scale != 50 || p.Error != "" {
		t.Fatalf("late result replaced current options: %+v", p)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	api.publishTextureMeasure(cancelled, next.Request, func(m *textureMeasurement) { m.err = "cancelled result" })
	if p := api.TextureMeasure(next.Request); p.Error != "" {
		t.Fatal("cancelled computation published a result")
	}
	api.texRead.mu.Unlock()
	locked = false
	p := waitTextureMeasure(t, api, next.Request)
	want, err := view.TexturePageOf(e.App, "all", 50)
	if err != nil || !reflect.DeepEqual(p.TexturePage, want) {
		t.Fatalf("background page differs from synchronous summary: %+v / %+v, %v", p.TexturePage, want, err)
	}
}

// Texture optimisation in the window: the page, a run in the background (the call returns at once, the
// pages answer, a second run is refused), its progress and its end as "textures" events, in
// the interface's words.
func TestTexturesBackground(t *testing.T) {
	t.Parallel()
	e, api, events := fixture(t)
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	p, err := api.Textures("plan", 50)
	if err != nil {
		t.Fatal(err)
	}
	p = waitTextureMeasure(t, api, p.Request)
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
	if _, err := api.LibraryDo(view.OpTexClean, []string{"#abc"}); err == nil {
		t.Fatal("library clean bypassed the active conversion")
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
	if !strings.Contains(ev, "converted 4 textures to DDS (1.0 MB written), 1 failed") {
		t.Fatalf("end: %s", ev)
	}
	if strings.Join(got, " ") != "textures optimize --plan --scale 50" {
		t.Fatalf("args %v", got)
	}
	// Mod library's selection
	release = make(chan struct{})
	close(release)
	api.LibraryDo(view.OpTextures, []string{"#abc"})
	wait(`"done":true`)
	if strings.Join(got, " ") != "textures optimize #abc" {
		t.Fatalf("refs: %v", got)
	}
}
