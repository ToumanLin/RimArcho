package loadout

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rimarcho/rimarcho/internal/source"
)

func reg() *Registry {
	return NewRegistry([]Candidate{
		{ID: "core", PackageID: "ludeon.rimworld", Kind: source.Official, Folder: "Core"},
		{ID: "ws-ce", PackageID: "ceteam.ce", Kind: source.Workshop, Folder: "2890901044"},
		{ID: "loc-ce", PackageID: "ceteam.ce", Kind: source.Local, Folder: "CE_local"},
		{ID: "loc-a1", PackageID: "dup.a", Kind: source.Local, Folder: "A1"},
		{ID: "loc-a2", PackageID: "dup.a", Kind: source.Local, Folder: "A2"},
		{ID: "ws-h", PackageID: "brrainz.harmony", Kind: source.Workshop, Folder: "2009463077"},
	})
}

func TestRegistrySteamSuffix(t *testing.T) {
	r := reg()
	if r.Token("loc-ce") != "ceteam.ce" || r.Token("ws-ce") != "ceteam.ce_steam" || r.Token("ws-h") != "brrainz.harmony" {
		t.Fatalf("tokens: %q %q %q", r.Token("loc-ce"), r.Token("ws-ce"), r.Token("ws-h"))
	}
	if r.Loadable("loc-a2") || len(r.Rejected) != 1 || r.Rejected[0].ID != "loc-a2" {
		t.Fatal("same-source duplicate must be rejected, folder order deciding")
	}
	for tok, want := range map[string]string{
		"ceteam.ce":             "loc-ce", // non-workshop wins the plain id
		"ceteam.ce_steam":       "ws-ce",
		"2009463077":            "ws-h", // folder name
		"Ludeon.RimWorld":       "core",
		"brrainz.harmony_steam": "ws-h", // local copy gone: strip suffix
		"gone.mod":              "",
	} {
		if got := r.Resolve(tok).Instance; got != want {
			t.Errorf("Resolve(%q) = %q, want %q", tok, got, want)
		}
	}
	l, res := r.FromTokens([]string{"ludeon.rimworld", "gone.mod", "brrainz.harmony", "2009463077"})
	if len(l) != 3 || l[1].Missing != "gone.mod" || res[3].Via != "duplicate" {
		t.Fatalf("list %+v res %+v", l, res)
	}
	if got := r.Tokens(l, nil); !slices.Equal(got, []string{"ludeon.rimworld", "gone.mod", "brrainz.harmony"}) {
		t.Fatalf("tokens %v", got)
	}
}

func TestRenderPreservesUnknown(t *testing.T) {
	orig := "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n<ModsConfigData>\r\n  <version>1.6.4871 rev591</version>\r\n  <activeMods>\r\n    <li>ludeon.rimworld</li>\r\n    <li>old.mod</li>\r\n  </activeMods>\r\n  <knownExpansions>\r\n    <li>ludeon.rimworld.royalty</li>\r\n  </knownExpansions>\r\n  <futureThing a=\"1\">keep me</futureThing>\r\n</ModsConfigData>\r\n"
	out, err := Render([]byte(orig), "1.7.4982 rev1280", []string{"ludeon.rimworld", "a&b.mod"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "\xef\xbb\xbf<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n<ModsConfigData>\r\n  <version>1.7.4982 rev1280</version>\r\n  <activeMods>\r\n    <li>ludeon.rimworld</li>\r\n    <li>a&amp;b.mod</li>\r\n  </activeMods>\r\n  <knownExpansions>\r\n    <li>ludeon.rimworld.royalty</li>\r\n  </knownExpansions>\r\n  <futureThing a=\"1\">keep me</futureThing>\r\n</ModsConfigData>\r\n"
	if string(out) != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}
	// Missing elements are inserted; a missing file is generated.
	out, _ = Render([]byte("<ModsConfigData><knownExpansions/></ModsConfigData>"), "1.7", []string{"x"}, nil)
	if !strings.Contains(string(out), "<version>1.7</version>") || !strings.Contains(string(out), "<li>x</li>") || !strings.Contains(string(out), "<knownExpansions/>") {
		t.Fatalf("insert: %s", out)
	}
	out, _ = Render(nil, "1.7", []string{"ludeon.rimworld"}, []string{"ludeon.rimworld.royalty"})
	p := filepath.Join(t.TempDir(), "ModsConfig.xml")
	os.WriteFile(p, out, 0o644)
	mc, err := ReadModsConfig(p)
	if err != nil || mc.Version != "1.7" || !slices.Equal(mc.Active, []string{"ludeon.rimworld"}) || !slices.Equal(mc.Known, []string{"ludeon.rimworld.royalty"}) {
		t.Fatalf("generated file round trip: %+v %v", mc, err)
	}
}

func TestWriteFileBackupAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ModsConfig.xml")
	os.WriteFile(p, []byte("old"), 0o644)
	bak, err := WriteFile(p, []byte("new"))
	if err != nil || bak != p+BackupSuffix {
		t.Fatal(bak, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Fatal("not replaced")
	}
	if b, _ := os.ReadFile(bak); string(b) != "old" {
		t.Fatal("backup wrong")
	}
	WriteFile(p, []byte("newer"))
	if b, _ := os.ReadFile(bak); string(b) != "new" {
		t.Fatal("only the latest backup is kept")
	}
	if _, err := os.Stat(p + ".rimarcho.tmp"); err == nil {
		t.Fatal("temp file left behind")
	}
}

func TestDiff(t *testing.T) {
	e := func(ids ...string) List {
		var l List
		for _, id := range ids {
			l = append(l, Entry{Instance: id})
		}
		return l
	}
	lin := func(id string) string {
		if strings.HasPrefix(id, "ce") {
			return "L-ce"
		}
		return "L-" + id
	}
	ch := Diff(e("core", "h", "a", "b", "c", "ce-ws"), e("core", "h", "c", "a", "b", "ce-local", "new"), lin)
	var got []string
	for _, c := range ch {
		got = append(got, c.Kind+":"+c.Entry.Instance)
	}
	want := []string{"moved:c", "replaced:ce-local", "added:new"}
	if !slices.Equal(got, want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
}
