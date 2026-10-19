package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func templ(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "mods", name)
}

func TestReadDeclTolerant(t *testing.T) {
	d := ReadDecl(templ("legacy"), false)
	if d.ParseError != "" {
		t.Fatal(d.ParseError)
	}
	if d.Name != "Old Mod" || !slices.Equal(d.Authors, []string{"Someone"}) || !slices.Equal(d.SupportedVersions, []string{"1.0"}) {
		t.Fatalf("decl = %+v", d)
	}
	if !d.PackageIDDerived || d.PackageID != "someone"+d.PackageID[len("someone"):] {
		t.Fatalf("derived package id = %q", d.PackageID)
	}
	if d.PackageID != lower(DerivePackageID("Someone", "Old Mod", "An old mod from 2018.")) {
		t.Fatalf("derived %q", d.PackageID)
	}

	v := ReadDecl(templ("versioned"), false)
	if v.PackageID != "tester.versioned" || v.PackageIDRaw != "Tester.Versioned" || v.ModVersion != "2.1" {
		t.Fatalf("versioned = %+v", v)
	}
	if !slices.Equal(v.Authors, []string{"Tester", "Friend"}) {
		t.Fatalf("authors = %q", v.Authors)
	}
	if deps := v.DependenciesByVersion["v1.6"]; len(deps) != 1 || deps[0].PackageID != "brrainz.harmony" {
		t.Fatalf("deps by version = %+v", v.DependenciesByVersion)
	}
	if !slices.Equal(v.AllDeps(), []string{"brrainz.harmony"}) || v.LoadFolders == "" {
		t.Fatalf("all deps %v, loadfolders %q", v.AllDeps(), v.LoadFolders)
	}

	h := ReadDecl(templ("harmony"), false)
	if h.PublishedFileID != "2009463077" || !slices.Equal(h.Authors, []string{"Andreas Pardeike"}) {
		t.Fatalf("harmony = %+v", h)
	}
	c := ReadDecl(templ("core"), true)
	if c.PackageID != "ludeon.rimworld" || c.PackageIDDerived {
		t.Fatalf("core = %+v", c)
	}
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func TestDerivePackageID(t *testing.T) {
	// No description → "none"; non-alphanumerics map to c%25+65.
	if got := DerivePackageID("Anonymous", "My Mod", ""); got != "Anonymousnone.MyHMod" {
		t.Fatalf("got %q", got)
	}
	// StableStringHash("a") = 23*31+97 = 810 → "810".
	if got := DerivePackageID("x", "y", "a"); got != "x810.y" {
		t.Fatalf("got %q", got)
	}
}

func TestIsSemantic(t *testing.T) {
	for p, want := range map[string]bool{
		"About/About.xml":               true,
		"about/Preview.png":             true,
		"LoadFolders.xml":               true,
		"Defs/ThingDefs/a.xml":          true,
		"1.6/Defs/a.xml":                true,
		"1.6/Patches/p.xml":             true,
		"Assemblies/Foo.dll":            true,
		"1.6/Assemblies/Foo.dll":        true,
		"Assemblies/Foo.pdb":            false,
		"Defs/readme.txt":               false,
		"Textures/Things/a.png":         false,
		"Languages/English/Keyed/a.xml": false,
		"Sounds/Defs.xml":               false,
	} {
		if IsSemantic(p) != want {
			t.Errorf("IsSemantic(%q) = %v", p, !want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	h := func(b byte) Hash { return Hash{b} }
	a := SemanticSet{"about/about.xml": h(1), "defs/a.xml": h(2), "defs/b.xml": h(3), "patches/p.xml": h(4)}
	same := SemanticSet{"about/about.xml": h(1), "defs/a.xml": h(2), "defs/b.xml": h(3), "patches/p.xml": h(4)}
	edited := SemanticSet{"about/about.xml": h(9), "defs/a.xml": h(9), "defs/b.xml": h(3), "patches/p.xml": h(4)}
	grown := SemanticSet{"about/about.xml": h(1), "defs/a.xml": h(2), "defs/b.xml": h(3), "patches/p.xml": h(4), "defs/c.xml": h(5)}
	other := SemanticSet{"about/about.xml": h(7), "defs/x.xml": h(8)}
	for _, c := range []struct {
		name string
		b    SemanticSet
		want float64
	}{{"same", same, 1}, {"edited (paths match)", edited, 1}, {"grown", grown, 0.8}, {"other", other, 0.2}} {
		if got := Similarity(a, c.b); got < c.want-1e-9 || got > c.want+1e-9 {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestComputeIncremental(t *testing.T) {
	dir := t.TempDir()
	write := func(p, s string) {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644)
	}
	write("About/About.xml", "<ModMetaData/>")
	write("Defs/a.xml", "<Defs/>")
	write("Textures/a.png", "png")
	write(".git/HEAD", "ref: refs/heads/main\n")
	write(".git/refs/heads/main", "abc123\n")
	write(".rimarcho/origin.json", "{}")
	fp, err := Compute(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.Files) != 3 || len(fp.Semantic) != 2 || fp.Hashed != 2 {
		t.Fatalf("files %d semantic %d hashed %d", len(fp.Files), len(fp.Semantic), fp.Hashed)
	}
	prev := map[string]File{}
	for _, f := range fp.Files {
		prev[lower(f.Rel)] = f
	}
	fp2, _ := Compute(dir, prev)
	if fp2.Hashed != 0 || fp2.Reused != 2 || fp2.SnapshotHash != fp.SnapshotHash {
		t.Fatalf("incremental: hashed %d reused %d", fp2.Hashed, fp2.Reused)
	}
	write("Textures/a.png", "png but bigger")
	fp3, _ := Compute(dir, prev)
	if fp3.SnapshotHash == fp.SnapshotHash {
		t.Fatal("size change of a texture must change the snapshot hash")
	}
	if gitHead(dir) != "abc123" {
		t.Fatalf("git head %q", gitHead(dir))
	}
}

func TestACF(t *testing.T) {
	root := t.TempDir()
	content := filepath.Join(root, "steamapps", "workshop", "content", "294100")
	os.MkdirAll(content, 0o755)
	os.WriteFile(filepath.Join(root, "steamapps", "workshop", "appworkshop_294100.acf"), []byte(`"AppWorkshop"
{
	"appid"		"294100"
	"WorkshopItemsInstalled"
	{
		"111"
		{
			"size"		"5"
			"timeupdated"		"1700000000"
			"manifest"		"42"
		}
	}
	"WorkshopItemDetails"
	{
		"222"
		{
			"manifest"		"43"
			"subscribedby"		"1"
		}
	}
}
`), 0o644)
	a := ReadACF(content)
	if a == nil || !a.Items["111"].Subscribed || a.Items["111"].Manifest != "42" || a.Items["111"].TimeUpdated != 1700000000 || !a.Items["222"].Subscribed {
		t.Fatalf("acf = %+v", a)
	}
	if a.Items["333"] != nil {
		t.Fatal("unknown item present")
	}
}
