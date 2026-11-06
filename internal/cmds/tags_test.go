package cmds_test

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// Whole-tag edits (07 §6.1, 16 §2.9 Tag manager): rename, merge, delete and the tag note,
// with tags.pinned kept in step.
func TestTagEdits(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddWorkshop("6100", testkit.Sample("tester.a", "Alpha"))
	e.AddWorkshop("6200", testkit.Sample("tester.b", "Beta"))
	e.AddWorkshop("6300", testkit.Sample("tester.c", "Gamma"))
	e.WriteModsConfig("1.6.4633 rev1261", "ludeon.rimworld")
	e.AddEnv(false)
	e.MustRun("scan")
	e.MustRun("config", "set", "tags.pinned", "visuals,framework")
	e.MustRun("tag", "add", "pkg:tester.a", "visuals", "framework")
	e.MustRun("tag", "add", "pkg:tester.b", "Visual")
	e.MustRun("tag", "add", "pkg:tester.c", "visual effects", "visuals")
	e.MustRun("tag", "note", "Visual", "hair and faces")
	e.MustRun("tag", "note", "visuals", "textures")

	tags := func() map[string]cmds.TagInfo {
		list, err := cmds.TagList(e.App)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]cmds.TagInfo{}
		for _, x := range list {
			out[x.Tag] = x
		}
		return out
	}

	// a dry run counts and writes nothing
	out := e.MustRun("tag", "merge", "Visual", "visual effects", "--into", "VISUALS", "--dry-run")
	if !strings.Contains(out, "dry run, tag merged: Visual, visual effects → visuals") || !strings.Contains(out, "2 mods change to visuals (1 already had it)") {
		t.Fatalf("merge dry run:\n%s", out)
	}
	if _, ok := tags()["Visual"]; !ok {
		t.Fatal("the dry run merged")
	}

	// merge: the existing target keeps its spelling, the notes join, the sources go
	e.MustRun("tag", "merge", "Visual", "visual effects", "--into", "VISUALS")
	got := tags()
	if v := got["visuals"]; v.N != 3 || v.Note != "textures\nhair and faces" || !v.Pinned {
		t.Fatalf("after merge: %+v", got)
	}
	if _, ok := got["Visual"]; ok {
		t.Fatalf("Visual left after the merge: %+v", got)
	}

	// rename onto another tag is a merge; a case-only rename keeps the pinned place
	if _, code := e.Run("tag", "rename", "visuals", "framework"); code == 0 {
		t.Fatal("rename onto an existing tag was accepted")
	}
	e.MustRun("tag", "rename", "visuals", "Visuals")
	if v := cmds.Setting(e.App, "tags.pinned"); v != "Visuals,framework" {
		t.Fatalf("pinned after a case-only rename: %q", v)
	}
	e.MustRun("tag", "rename", "Visuals", "looks")
	if v := cmds.Setting(e.App, "tags.pinned"); v != "looks,framework" {
		t.Fatalf("pinned after a rename: %q", v)
	}
	if v := tags()["looks"]; v.N != 3 || !strings.HasPrefix(v.Note, "textures") {
		t.Fatalf("renamed tag: %+v", v)
	}

	// delete: the tag, its note and its pinned place go; mods stay
	out = e.MustRun("tag", "delete", "looks")
	if !strings.Contains(out, "3 mods lose the tag") {
		t.Fatalf("delete:\n%s", out)
	}
	if _, ok := tags()["looks"]; ok || cmds.Setting(e.App, "tags.pinned") != "framework" {
		t.Fatalf("after delete: %+v, pinned %q", tags(), cmds.Setting(e.App, "tags.pinned"))
	}
	n, _ := e.App.Store.TagNotes()
	if _, ok := n["looks"]; ok {
		t.Fatal("the deleted tag's note is left")
	}
	// deleting the last pinned tag leaves none pinned, not the preset
	e.MustRun("tag", "delete", "framework")
	if p := cmds.PinnedTagList(e.App); len(p) != 0 {
		t.Fatalf("pinned after deleting every pinned tag: %v", p)
	}

	// a missing tag is an error, the note can be cleared
	if _, code := e.Run("tag", "delete", "nope"); code == 0 {
		t.Fatal("deleted a tag that does not exist")
	}
	e.MustRun("tag", "add", "pkg:tester.a", "QoL")
	e.MustRun("tag", "note", "qol", "always on")
	if out := e.MustRun("tag", "list"); !strings.Contains(out, "QoL") || !strings.Contains(out, "note: always on") {
		t.Fatalf("tag list:\n%s", out)
	}
	e.MustRun("tag", "note", "QoL", "--clear")
	if v := tags()["QoL"]; v.Note != "" {
		t.Fatalf("note not cleared: %+v", v)
	}
}
