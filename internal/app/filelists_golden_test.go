package app_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/modchanges"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

var updateFileLists = flag.Bool("update-filelists", false, "rewrite testdata/store/filelists-scenario.txt")

// 04 §4 (v28): everything read from snapshot file lists is the same as when every file was a
// row. The golden file was written by this test on the v27 code (dev/1.0 57a7643, before the
// blocks); v28 must reproduce it byte for byte: the change history and its file diffs, each
// snapshot's file list, the log lookups of textures and DLLs, and what maintenance prunes.
func TestFileListsMatchRows(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	e.AddEnv(false)
	files := map[string]string{
		"About/Preview.png":                              "preview",
		"LoadFolders.xml":                                "<loadFolders><v1.6><li>/</li><li>1.6</li></v1.6></loadFolders>\n",
		"Defs/ThingDefs/Things.xml":                      "<Defs><ThingDef><defName>A_Thing</defName></ThingDef></Defs>\n",
		"1.6/Defs/Extra.xml":                             "<Defs><ThingDef><defName>A_Extra</defName></ThingDef></Defs>\n",
		"Patches/A.xml":                                  "<Patch/>\n",
		"Assemblies/A.dll":                               "dll-a",
		"Libs/Native.DLL":                                "native",
		"Textures/Things/A/a_north.png":                  strings.Repeat("n", 40),
		"Textures/Things/A/a_south.PNG":                  strings.Repeat("s", 41),
		"Textures/中文/图.png":                              strings.Repeat("z", 12),
		"1.6/Textures/UI/Icon.png":                       strings.Repeat("i", 9),
		"Sounds/Click.ogg":                               strings.Repeat("o", 300),
		"Languages/English/Keyed/A.xml":                  "<LanguageData/>\n",
		"Languages/ChineseSimplified (简体中文)/Keyed/A.xml": "<LanguageData/>\n",
		"Languages/English/DefInjected/ThingDef/A.xml":   "<LanguageData/>\n",
		"News/readme.txt":                                "hello",
		"Textures/Things/A/a_east.png.meta":              "meta",
	}
	a := testkit.Sample("test.alpha", "Alpha")
	a.Files = files
	dirA := e.AddWorkshop("1001", a)
	e.AddWorkshop("1002", testkit.Sample("test.beta", "Beta"))
	e.WriteMod(filepath.Join(e.Game, "Mods", "Local"), testkit.Sample("test.local", "Local"))
	scan := func() {
		e.Advance(time.Hour)
		e.MustRun("scan")
	}
	scan()
	write := func(rel, content string) { e.WriteFile(filepath.Join(dirA, filepath.FromSlash(rel)), content) }
	// Five updates: textures added, grown, removed; Defs and DLLs changed; Languages renamed.
	for round := 1; round <= 5; round++ {
		write(fmt.Sprintf("Textures/Things/A/new_%d.png", round), strings.Repeat("p", 20+round))
		write("Textures/Things/A/a_north.png", strings.Repeat("n", 40+round))
		write("Defs/ThingDefs/Things.xml", fmt.Sprintf("<Defs><ThingDef><defName>A_Thing%d</defName></ThingDef></Defs>\n", round))
		switch round {
		case 2:
			e.Remove(filepath.Join(dirA, "Textures", "中文"))
			write("Assemblies/B.dll", "dll-b")
		case 3:
			write("Libs/Native.DLL", "native-3")
			e.Remove(filepath.Join(dirA, "Sounds"))
		case 4:
			e.Move(filepath.Join(dirA, "Languages", "English"), filepath.Join(dirA, "Languages", "English (US)"))
		}
		e.SetTimeUpdated("1001", int64(1000+round))
		scan()
	}
	// Same content, newer mtimes: the stored list is refreshed in place.
	later := e.Now().Add(48 * time.Hour)
	os.Chtimes(filepath.Join(dirA, "Defs", "ThingDefs", "Things.xml"), later, later)
	scan()
	// Same content, new external state: the list is copied to the new snapshot.
	e.SetManifest("1001", 77)
	scan()

	// The comparison means something only if the lists really are split.
	var blocks, plain, dlls int
	e.App.Store.DB.QueryRow(`SELECT (SELECT count(*) FROM snapshot_list),
		(SELECT count(*) FROM snapshot_file WHERE hash IS NULL AND rel_path NOT LIKE '%.dll'),
		(SELECT count(*) FROM snapshot_file WHERE hash IS NULL AND rel_path LIKE '%native.dll')`).Scan(&blocks, &plain, &dlls)
	if blocks == 0 || plain != 0 || dlls == 0 {
		t.Fatalf("blocks %d, non-semantic rows %d, DLL rows outside Assemblies %d", blocks, plain, dlls)
	}

	var out strings.Builder
	section := func(name string) { fmt.Fprintf(&out, "\n## %s\n", name) }
	section("changes --library --all")
	out.WriteString(e.Scrub(e.MustRun("changes", "--library", "--all")))
	section("changes test.alpha")
	out.WriteString(e.Scrub(e.MustRun("changes", "pkg:test.alpha")))

	s := e.App.Store
	type pair struct{ before, after int64 }
	var updates []pair
	rows, err := s.DB.Query(`SELECT coalesce(before_snapshot,0), coalesce(after_snapshot,0) FROM change_event WHERE kind='updated' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var p pair
		rows.Scan(&p.before, &p.after)
		updates = append(updates, p)
	}
	rows.Close()
	tx := modchanges.English(time.UTC)
	files2 := func(p pair) {
		fmt.Fprintf(&out, "files %d → %d\n", p.before, p.after)
		for _, l := range modchanges.Files(e.App, tx, p.before, p.after) {
			fmt.Fprintf(&out, "  %s\n", l)
		}
	}
	section("file diffs")
	for _, p := range updates {
		files2(p)
	}
	snapshots := func() []int64 {
		var ids []int64
		rows, err := s.DB.Query(`SELECT id FROM snapshot ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			ids = append(ids, id)
		}
		return ids
	}
	dump := func() {
		for _, id := range snapshots() {
			fs, err := s.SnapshotFiles(id)
			if err != nil {
				t.Fatal(err)
			}
			if fs == nil {
				fmt.Fprintf(&out, "snapshot %d: none\n", id)
				continue
			}
			keys := make([]string, 0, len(fs))
			for k := range fs {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Fprintf(&out, "snapshot %d: %d files\n", id, len(keys))
			for _, k := range keys {
				f := fs[k]
				fmt.Fprintf(&out, "  %s | %s %d hash=%v\n", k, f.Rel, f.Size, f.Hash != nil)
			}
		}
	}
	section("snapshot files")
	dump()
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	section("instance files")
	for _, like := range []string{"%Textures/%", "%a.dll", "%native.dll", "%.png", "%/a\\_%", "Languages/_%/Keyed/%", "%中文%", "%"} {
		m, err := s.InstanceFiles(st.Ix.Env.ID, like)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(m))
		for id := range m {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		fmt.Fprintf(&out, "like %q\n", like)
		for _, id := range ids {
			fmt.Fprintf(&out, "  %s: %s\n", id, strings.Join(m[id], ", "))
		}
	}
	section("maintenance")
	r, err := e.App.Maintain(context.Background(), false, true)
	fmt.Fprintf(&out, "dry run: files=%d err=%v\n", r.Files, err)
	r, err = e.App.Maintain(context.Background(), false, false)
	fmt.Fprintf(&out, "run: files=%d err=%v\n", r.Files, err)
	dump()
	for _, p := range updates {
		files2(p)
	}

	got := out.String()
	path := filepath.Join("..", "..", "testdata", "store", "filelists-scenario.txt")
	if *updateFileLists {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		os.WriteFile(filepath.Join(t.TempDir(), "got.txt"), []byte(got), 0o644)
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range min(len(gl), len(wl)) {
			if gl[i] != wl[i] {
				t.Fatalf("line %d differs:\n got %q\nwant %q", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("output has %d lines, want %d", len(gl), len(wl))
	}
}
