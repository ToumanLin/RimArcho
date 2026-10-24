package modsearch

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

func fixture(t testing.TB, files map[string]string) Mod {
	t.Helper()
	root := t.TempDir()
	for path, text := range files {
		path = filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return Mod{Root: root, Pos: 1, Ref: "#one", PackageID: "test.mod", Name: "Test", Copy: "local Test"}
}
func search(t testing.TB, mods []Mod, o Options) *Result {
	t.Helper()
	if o.Pattern == "" {
		o.Pattern = "needle"
	}
	if o.MM == "" {
		o.MM = "1.6"
	}
	r, err := Search(context.Background(), mods, o)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func paths(r *Result) []string {
	var out []string
	for _, mod := range r.Mods {
		for _, file := range mod.Files {
			out = append(out, file.Rel)
		}
	}
	return out
}
func matches(r *Result) []Match {
	var out []Match
	for _, mod := range r.Mods {
		for _, file := range mod.Files {
			out = append(out, file.Matches...)
		}
	}
	return out
}

func TestLoadFoldersAndScope(t *testing.T) {
	m := fixture(t, map[string]string{
		"1.5/Defs/old.xml": "needle", "1.6/dEfS/exact.XML": "needle",
		"Common/Defs/common.xml": "needle", "Defs/root.xml": "needle",
		"About/About.xml": "needle", "About/extra.xml": "needle", "About/ignored.txt": "needle",
		"SimplifiedMode/PatchesReplace.xml": "needle", "Other/Defs/stray.xml": "needle",
		"Defs/ignored.txt": "needle", "root.xml": "needle", "Languages/English/a.xml": "needle",
	})
	r := search(t, []Mod{m}, Options{})
	want := []string{"1.6/dEfS/exact.XML", "Common/Defs/common.xml", "About/About.xml", "About/extra.xml", "Defs/root.xml"}
	if !reflect.DeepEqual(paths(r), want) {
		t.Fatalf("paths %v, want %v", paths(r), want)
	}
	if !reflect.DeepEqual(r.Mods[0].Folders, []string{"1.6", "Common", "/"}) {
		t.Fatal(r.Mods[0].Folders)
	}
	r = search(t, []Mod{m}, Options{MM: "1.7"})
	if r.Mods[0].Folders[0] != "1.6" {
		t.Fatal(r.Mods[0].Folders)
	}
	r = search(t, []Mod{m}, Options{MM: "1.4"})
	if r.Mods[0].Folders[0] != "1.5" {
		t.Fatal(r.Mods[0].Folders)
	}
}

func TestConditionalLoadFolders(t *testing.T) {
	m := fixture(t, map[string]string{
		"Defs/root.xml": "needle", "About/About.xml": "needle", "1.6/Defs/base.xml": "needle",
		"Compat/Patches/a.xml": "needle", "Absent/Patches/b.xml": "needle", "Old/Defs/old.xml": "needle",
	})
	m.LoadFoldersXML = `<loadFolders><v1.5><li>Old</li></v1.5><v1.6><li>1.6</li><li IfModActive="Other.Mod_steam">Compat</li><li IfModNotActive="other.mod">Absent</li></v1.6></loadFolders>`
	for _, enabled := range []bool{false, true} {
		r := search(t, []Mod{m}, Options{Active: func(pkg string) bool {
			if pkg != "other.mod" {
				t.Errorf("package %q", pkg)
			}
			return enabled
		}})
		folder := "Absent"
		if enabled {
			folder = "Compat"
		}
		if !reflect.DeepEqual(r.Mods[0].Folders, []string{folder, "1.6"}) {
			t.Fatal(r.Mods[0].Folders)
		}
		if r.Matches != 3 || strings.Contains(strings.Join(paths(r), ","), "Defs/root.xml") {
			t.Fatal(paths(r), r.Matches)
		}
		if r.Mods[0].Files[2].Folder != "/" {
			t.Fatal(r.Mods[0].Files)
		}
	}
	r := search(t, []Mod{m}, Options{}) // nil Active is an empty loadout.
	if r.Mods[0].Folders[0] != "Absent" {
		t.Fatal(r.Mods[0].Folders)
	}
	r = search(t, []Mod{m}, Options{MM: "1.7"})
	if r.Mods[0].Folders[1] != "1.6" {
		t.Fatal(r.Mods[0].Folders)
	}
}

func TestShadowAndAllVersions(t *testing.T) {
	m := fixture(t, map[string]string{
		"1.5/Defs/same.xml": "needle old", "1.6/Defs/Same.xml": "needle exact",
		"Common/Defs/SAME.xml": "needle common", "Defs/same.xml": "needle root",
		"Special/Defs/same.xml": "needle special", "Disabled/Patches/a.xml": "needle disabled",
	})
	r := search(t, []Mod{m}, Options{})
	if r.Matches != 1 || !reflect.DeepEqual(paths(r), []string{"1.6/Defs/Same.xml"}) {
		t.Fatal(paths(r), r.Matches)
	}
	m.LoadFoldersXML = `<loadFolders><v1.5><li>Special</li></v1.5><v1.6><li>/</li><li>1.6</li><li IfModActive="missing">Disabled</li></v1.6></loadFolders>`
	r = search(t, []Mod{m}, Options{AllVersions: true})
	if r.Matches != 6 {
		t.Fatal(paths(r), r.Matches)
	}
	if !reflect.DeepEqual(r.Mods[0].Folders, []string{"1.5", "1.6", "Common", "/", "Special", "Disabled"}) {
		t.Fatal(r.Mods[0].Folders)
	}
	for _, file := range r.Mods[0].Files {
		if file.Folder == "" {
			t.Fatal(file)
		}
	}
}

func TestLanguagesAndKinds(t *testing.T) {
	m := fixture(t, map[string]string{
		"Defs/a.xml": "needle", "lAnGuAgEs/English/Keyed/a.xml": "needle",
		"lAnGuAgEs/English/Strings/a.txt": "needle", "lAnGuAgEs/French/Keyed/a.xml": "needle",
		"lAnGuAgEs/English/a.json": "needle", "lAnGuAgEs/root.xml": "needle",
	})
	if r := search(t, []Mod{m}, Options{}); r.Matches != 1 {
		t.Fatal(paths(r))
	}
	if r := search(t, []Mod{m}, Options{Kinds: Languages}); r.Matches != 4 {
		t.Fatal(paths(r))
	}
	r := search(t, []Mod{m}, Options{Langs: []string{"eNGLISH"}})
	if r.Matches != 3 || !strings.Contains(strings.Join(paths(r), ","), "Strings/a.txt") {
		t.Fatal(paths(r))
	}
	for _, input := range []string{"defs,PATCHES, About", "all", "Textures,sounds,ASSEMBLIES", "languages"} {
		if k, err := ParseKinds(input); err != nil || k == 0 {
			t.Fatal(input, k, err)
		}
	}
	if k, _ := ParseKinds("all"); k != allKinds {
		t.Fatal(k)
	}
	for _, input := range []string{"", "def", "defs,", "defs,nope"} {
		if _, err := ParseKinds(input); err == nil {
			t.Fatal("accepted", input)
		}
	}
}

func TestDuplicateCopies(t *testing.T) {
	a := fixture(t, map[string]string{"Defs/a.xml": "needle"})
	b := fixture(t, map[string]string{"Defs/b.xml": "needle"})
	a.Ref, a.Copy, a.Pos = "#local", "local Foo", 0
	b.Ref, b.Copy, b.Pos = "#workshop", "workshop ws:123", 8
	r := search(t, []Mod{b, a}, Options{})
	if r.ModsSearched != 2 || r.ModsMatched != 2 || r.Mods[0].Mod != b || r.Mods[1].Mod != a {
		t.Fatal(r.Mods)
	}
}

// This FS has no corresponding files on disk, so resolution cannot accidentally use os.
type memoryFS struct {
	root  string
	tree  fstest.MapFS
	reads atomic.Int32
}

func (m *memoryFS) rel(path string) string {
	rel, _ := filepath.Rel(m.root, path)
	return filepath.ToSlash(rel)
}
func (m *memoryFS) ReadDir(path string) ([]fs.DirEntry, error) {
	return fs.ReadDir(m.tree, m.rel(path))
}
func (m *memoryFS) ReadFile(path string) ([]byte, error) {
	m.reads.Add(1)
	return fs.ReadFile(m.tree, m.rel(path))
}
func TestFSAndBinaryPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-on-disk")
	mem := &memoryFS{root: root, tree: fstest.MapFS{
		"1.6/Textures/Things/Item/Foo.png": {Data: []byte("no content search")},
		"Textures/Things/Item/Foo.png":     {Data: []byte("shadowed")},
		"Common/Sounds/Foo.ogg":            {}, "Lunar/cOmPoNeNtS/Foo.dll": {},
		"Assemblies/Foo.dll": {}, "Assemblies/Foo.pdb": {}, "1.5/Defs/a.xml": {Data: []byte("needle")},
	}}
	r := search(t, []Mod{{Root: root}}, Options{Pattern: "foo", Kinds: Textures | Sounds | Assemblies, FS: mem})
	if r.Matches != 4 || mem.reads.Load() != 0 {
		t.Fatal(paths(r), mem.reads.Load())
	}
	for _, hit := range matches(r) {
		if hit.Line != 0 || hit.Col < 1 {
			t.Fatal(hit)
		}
	}
	r = search(t, []Mod{{Root: root}}, Options{Pattern: "Things/Item/Foo.png", Kinds: Textures, FS: mem})
	if r.Matches != 1 || matches(r)[0].Text != "Things/Item/Foo.png" {
		t.Fatal(r)
	}
	// two reads: the search, then the enclosing-def pass for the shown file
	r = search(t, []Mod{{Root: root}}, Options{MM: "1.5", FS: mem})
	if r.Matches != 1 || mem.reads.Load() != 2 || r.Mods[0].Folders[0] != "1.5" {
		t.Fatal(r, mem.reads.Load())
	}
}

func TestMatching(t *testing.T) {
	m := fixture(t, map[string]string{"Defs/a.xml": "xxFOO Foo foo\nfoo_bar\nfood\n(foo)\nFOO\nfoo42\n"})
	cases := []struct {
		name  string
		o     Options
		lines []int
	}{
		{"literal", Options{Pattern: "foo"}, []int{1, 2, 3, 4, 5, 6}},
		{"word", Options{Pattern: "foo", Word: true}, []int{1, 4, 5}},
		{"case", Options{Pattern: "FOO", CaseSensitive: true, Word: true}, []int{5}},
		{"regex", Options{Pattern: "f.o", Regex: true, Word: true}, []int{1, 4, 5}},
		{"regex-case", Options{Pattern: "FOO", Regex: true, CaseSensitive: true, Word: true}, []int{5}},
		{"prefix", Options{Pattern: "foo[0-9]+", Regex: true, CaseSensitive: true}, []int{6}},
		{"miss", Options{Pattern: "absent[0-9]+", Regex: true, CaseSensitive: true}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.o.PerFile = 20
			r := search(t, []Mod{m}, tc.o)
			var lines []int
			for _, hit := range matches(r) {
				lines = append(lines, hit.Line)
			}
			if !reflect.DeepEqual(lines, tc.lines) || r.Matches != len(tc.lines) {
				t.Fatal(lines, r.Matches)
			}
		})
	}
	r := search(t, []Mod{m}, Options{Pattern: "foo", CaseSensitive: true, PerFile: 20})
	if matches(r)[0].Col != 11 {
		t.Fatal(matches(r)[0])
	}
}

func TestDefRolesAndWithin(t *testing.T) {
	m := fixture(t, map[string]string{
		"Defs/a.xml": `<Defs>
<ThingDef Name = 'Target' Abstract="True">
 <label>Target</label>
</ThingDef>
<ThingDef ParentName="Target">
 <defName>Target</defName>
 <thingClass>Target</thingClass>
</ThingDef>
<ThingDef><defName>Other</defName>
 <label>Target</label></ThingDef>
</Defs>`,
		"Patches/a.xml": `<Patch><Operation Class="PatchOperationSequence">
<operations><li><xpath>Defs/ThingDef[defName="Target"]</xpath></li></operations>
<value>Target</value>
</Operation><Operation><value>Target</value><xpath>second</xpath></Operation></Patch>`,
		"Languages/English/a.xml": "<Target.label>Target</Target.label>",
		"About/About.xml":         "Target",
	})
	r := search(t, []Mod{m}, Options{Pattern: "Target", Def: true, Regex: true, PerFile: 20})
	want := []Match{
		{Line: 2, Role: "abstract", Within: "ThingDef@Target"},
		{Line: 3, Role: "ref", Within: "ThingDef@Target"},
		{Line: 5, Role: "child", Within: "ThingDef/Target"},
		{Line: 6, Role: "def", Within: "ThingDef/Target"},
		{Line: 7, Role: "ref", Within: "ThingDef/Target"},
		{Line: 10, Role: "ref", Within: "ThingDef/Other"},
		{Line: 2, Role: "patch", Within: `Defs/ThingDef[defName="Target"]`},
		{Line: 3, Role: "patch", Within: `Defs/ThingDef[defName="Target"]`},
		{Line: 4, Role: "patch-value", Within: "second"},
	}
	got := matches(r)
	if len(got) != len(want) {
		t.Fatalf("%+v", got)
	}
	for i, w := range want {
		if got[i].Line != w.Line || got[i].Role != w.Role || got[i].Within != w.Within {
			t.Errorf("hit %d: %+v, want %+v", i, got[i], w)
		}
	}
	r = search(t, []Mod{m}, Options{Pattern: "Target", PerFile: 20})
	for _, f := range r.Mods[0].Files {
		for _, hit := range f.Matches {
			if hit.Role != "" {
				t.Fatal(hit)
			}
			if (f.Kind == Defs || f.Kind == Patches) && hit.Within == "" {
				t.Fatal(hit)
			}
		}
	}
	r = search(t, []Mod{m}, Options{Pattern: "Target", Def: true, Kinds: Languages | About})
	if r.Matches != 2 || matches(r)[0].Role != "ref" || matches(r)[1].Role != "translation" {
		t.Fatal(matches(r))
	}
	r = search(t, []Mod{m}, Options{Pattern: "target", Def: true})
	if r.Matches != 0 {
		t.Fatal(matches(r))
	}
}

func TestPagingAndOrder(t *testing.T) {
	m := fixture(t, map[string]string{
		"Defs/z.xml": "needle\nneedle needle\nneedle\nneedle\n", "Defs/A.xml": "needle\nneedle\nneedle\n",
		"Defs/a-dir/z.xml": "needle", "Defs/a-dir/deep/a.xml": "needle", "Defs/b-dir/a.xml": "needle", "Patches/a.xml": "needle",
	})
	want := []string{"Defs/A.xml", "Defs/z.xml", "Defs/a-dir/z.xml", "Defs/b-dir/a.xml", "Defs/a-dir/deep/a.xml", "Patches/a.xml"}
	for _, workers := range []int{1, 2, 16} {
		r := search(t, []Mod{m}, Options{Workers: workers, PerFile: 2, Limit: 100})
		if !reflect.DeepEqual(paths(r), want) {
			t.Fatal(paths(r))
		}
		if r.Matches != 11 || r.Shown != 8 || r.FilesMatched != 6 || r.FilesSearched != 6 || r.ModsMatched != 1 || !r.Truncated {
			t.Fatalf("%+v", r)
		}
		page := search(t, []Mod{m}, Options{Workers: workers, PerFile: 2, Offset: 1, Limit: 3})
		if page.Matches != 11 || page.Shown != 3 || page.Offset != 1 || len(paths(page)) != 2 || page.Mods[0].Files[1].Count != 4 {
			t.Fatalf("%+v", page)
		}
		hits := matches(page)
		if hits[0].Line != 2 || hits[1].Line != 1 || hits[2].Line != 2 {
			t.Fatal(hits)
		}
	}
	r := search(t, []Mod{m}, Options{Offset: 200})
	if r.Shown != 0 || len(paths(r)) != 0 || r.Matches != 11 || r.ModsMatched != 1 {
		t.Fatal(r)
	}
	r = search(t, []Mod{m}, Options{})
	if r.Shown != 11 || r.Truncated {
		t.Fatal(r)
	}
}

func TestEncodingAndLongLines(t *testing.T) {
	text := "<Defs><ThingDef><defName>needle</defName>\n<label>" + strings.Repeat("界", 100) + "needle</label></ThingDef></Defs>"
	m := fixture(t, map[string]string{"Defs/a.xml": text})
	var b []byte
	b = binary.LittleEndian.AppendUint16(b, 0xfeff)
	for _, u := range utf16.Encode([]rune(text)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	if err := os.WriteFile(filepath.Join(m.Root, "Defs", "a.xml"), b, 0644); err != nil {
		t.Fatal(err)
	}
	r := search(t, []Mod{m}, Options{})
	if r.Matches != 2 {
		t.Fatal(r)
	}
	for _, hit := range matches(r) {
		if !utf8.ValidString(hit.Text) || len(hit.Text) > 160 || hit.Within != "ThingDef/needle" {
			t.Fatal(hit)
		}
	}
	// Searching is independent of Scanner's line-length limit.
	m = fixture(t, map[string]string{"Defs/a.xml": strings.Repeat("x", 5<<20) + "needle\nneedle"})
	if r := search(t, []Mod{m}, Options{}); r.Matches != 2 || matches(r)[1].Line != 2 {
		t.Fatal(r)
	}
}

func TestErrorsAndCancellation(t *testing.T) {
	for _, o := range []Options{{}, {Pattern: "[", Regex: true}, {Pattern: "x", Offset: -1}, {Pattern: "x", Limit: -1}, {Pattern: "x", PerFile: -1}, {Pattern: "x", Workers: -1}, {Pattern: "x", Kinds: 1 << 15}} {
		if _, err := Search(context.Background(), nil, o); err == nil {
			t.Fatal("accepted", o)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Search(ctx, nil, Options{Pattern: "x"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r := search(t, nil, Options{})
	if r.Matches != 0 || r.FilesSearched != 0 || r.Truncated {
		t.Fatal(r)
	}
	m := fixture(t, map[string]string{"Defs/a.xml": "needle"})
	failing := &errorFS{FS: OS}
	if r := search(t, []Mod{m}, Options{FS: failing}); r.Unreadable != 1 || !strings.Contains(r.FirstError, filepath.Join(m.Root, "Defs", "a.xml")) || r.Matches != 0 || r.FilesSearched != 1 {
		t.Fatal(r)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancelling := &cancelFS{FS: OS, cancel: cancel}
	if _, err := Search(ctx, []Mod{m}, Options{Pattern: "needle", FS: cancelling}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type errorFS struct{ FS }

func (*errorFS) ReadFile(p string) ([]byte, error) {
	return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}
}

type cancelFS struct {
	FS
	cancel context.CancelFunc
}

func (f *cancelFS) ReadFile(path string) ([]byte, error) { f.cancel(); return f.FS.ReadFile(path) }

func BenchmarkSearch5000(b *testing.B) {
	files := make(map[string]string, 5000)
	for i := 0; i < 5000; i++ {
		files[fmt.Sprintf("1.6/Defs/group%02d/file%04d.xml", i%50, i)] = fmt.Sprintf("<Defs><ThingDef><defName>Item%d</defName><label>small fixture</label></ThingDef></Defs>", i)
	}
	mod := fixture(b, files)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r := search(b, []Mod{mod}, Options{Pattern: "Item42"})
		if r.FilesSearched != 5000 || r.Matches != 111 {
			b.Fatal(r.FilesSearched, r.Matches)
		}
	}
}
func TestPagingDefaults(t *testing.T) {
	files := map[string]string{"Defs/00.xml": strings.Repeat("needle\n", 9), "Defs/miss.xml": "no match"}
	for i := 1; i <= 55; i++ {
		files[fmt.Sprintf("Defs/%02d.xml", i)] = "needle"
	}
	mod := fixture(t, files)
	r := search(t, []Mod{mod}, Options{})
	if r.Matches != 64 || r.FilesMatched != 56 || r.FilesSearched != 57 || r.Shown != 50 || !r.Truncated {
		t.Fatalf("%+v", r)
	}
	if len(r.Mods[0].Files[0].Matches) != 5 || r.Mods[0].Files[0].Count != 9 {
		t.Fatal(r.Mods[0].Files[0])
	}
	r = search(t, []Mod{mod}, Options{Offset: 58, Limit: 10})
	if r.Shown != 2 || !reflect.DeepEqual(paths(r), []string{"Defs/54.xml", "Defs/55.xml"}) {
		t.Fatal(paths(r), r.Shown)
	}
}

func TestContextEdgeCases(t *testing.T) {
	mod := fixture(t, map[string]string{
		"Defs/a.xml": `<?xml version="1.0" encoding="utf-16"?><Defs>
<!-- Target is a comment -->
<ThingDef Name="Base"
 ParentName = 'Target'>
 <defName>Other</defName><label>&unknown; Target</label>
</ThingDef>
<ThingDef><defName>Third</defName><label>Target</label></ThingDef>
<ThingDef Name="Incomplete"><label>Target`,
		"Patches/a.xml": "<Patch><Operation><value>Target</value><xpath>" + strings.Repeat("界", 70) + "</xpath><xpath>ignored</xpath></Operation></Patch>",
	})
	r := search(t, []Mod{mod}, Options{Pattern: "Target", Def: true, PerFile: 20})
	hits := matches(r)
	if len(hits) != 6 {
		t.Fatalf("%+v", hits)
	}
	if hits[0].Role != "ref" || hits[0].Within != "" {
		t.Fatal(hits[0])
	}
	if hits[1].Role != "child" || hits[1].Within != "ThingDef/Other" {
		t.Fatal(hits[1])
	}
	if hits[2].Within != "ThingDef/Other" || hits[3].Within != "ThingDef/Third" || hits[4].Within != "ThingDef@Incomplete" {
		t.Fatal(hits)
	}
	if !utf8.ValidString(hits[5].Within) || len(hits[5].Within) > 120 || !strings.HasSuffix(hits[5].Within, "...") {
		t.Fatal(hits[5])
	}
}

func TestAllVersionsIgnoresConditions(t *testing.T) {
	mod := fixture(t, map[string]string{"Conditional/Defs/a.xml": "needle"})
	mod.LoadFoldersXML = `<loadFolders><v1.6><li IfModActive="other.mod">Conditional</li></v1.6></loadFolders>`
	r := search(t, []Mod{mod}, Options{AllVersions: true, Active: func(string) bool { t.Fatal("AllVersions evaluated Active"); return false }})
	if r.Matches != 1 || r.Mods[0].Files[0].Folder != "Conditional" {
		t.Fatal(r)
	}
}

type dirErrorFS struct{ FS }

func (*dirErrorFS) ReadDir(string) ([]fs.DirEntry, error) { return nil, fs.ErrPermission }
func TestDirectoryError(t *testing.T) {
	root := t.TempDir()
	if r := search(t, []Mod{{Root: root}}, Options{FS: &dirErrorFS{FS: OS}}); r.Unreadable != 1 || !strings.HasPrefix(r.FirstError, root) {
		t.Fatal(r)
	}
}

type faultFS struct {
	FS
	badFile, badDir string
}

func (f *faultFS) ReadFile(path string) ([]byte, error) {
	if path == f.badFile {
		return nil, fs.ErrNotExist
	}
	return f.FS.ReadFile(path)
}
func (f *faultFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if path == f.badDir {
		return nil, fs.ErrPermission
	}
	return f.FS.ReadDir(path)
}
func TestUnreadableKeepsOtherHits(t *testing.T) {
	mod := fixture(t, map[string]string{"Defs/good.xml": "needle", "Defs/vanished.xml": "needle", "Defs/locked/a.xml": "needle", "Patches/good.xml": "needle"})
	fault := &faultFS{FS: OS, badFile: filepath.Join(mod.Root, "Defs", "vanished.xml"), badDir: filepath.Join(mod.Root, "Defs", "locked")}
	r := search(t, []Mod{mod}, Options{FS: fault})
	if r.Matches != 2 || r.FilesMatched != 2 || r.FilesSearched != 3 || r.Unreadable != 2 || !strings.HasPrefix(r.FirstError, fault.badDir) {
		t.Fatalf("%+v", r)
	}
}

type barrierFS struct {
	FS
	ctx     context.Context
	roots   map[string]bool
	arrived atomic.Int32
	ready   chan struct{}
}

func (f *barrierFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if f.roots[path] {
		if f.arrived.Add(1) == 2 {
			close(f.ready)
		}
		select {
		case <-f.ready:
		case <-f.ctx.Done():
			return nil, f.ctx.Err()
		}
	}
	return f.FS.ReadDir(path)
}
func TestListingsRunInParallel(t *testing.T) {
	a := fixture(t, map[string]string{"Defs/a.xml": "needle"})
	b := fixture(t, map[string]string{"Defs/b.xml": "needle"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	f := &barrierFS{FS: OS, ctx: ctx, roots: map[string]bool{a.Root: true, b.Root: true}, ready: make(chan struct{})}
	r, err := Search(ctx, []Mod{a, b}, Options{Pattern: "needle", MM: "1.6", Workers: 2, FS: f})
	if err != nil {
		t.Fatal("mod listings did not overlap:", err)
	}
	if !reflect.DeepEqual(paths(r), []string{"Defs/a.xml", "Defs/b.xml"}) || r.Mods[0].Root != a.Root || r.Mods[1].Root != b.Root {
		t.Fatal(r)
	}
}

func TestDefSummaryBeforeCapsAndPaging(t *testing.T) {
	mod := fixture(t, map[string]string{
		"Defs/a.xml":    "<Defs><ThingDef ParentName=\"Target\"><defName>Target</defName><label>Target</label></ThingDef>\n<ThingDef Name=\"Target\"><label>Target</label></ThingDef></Defs>",
		"Patches/a.xml": "<Patch><Operation><xpath>Target</xpath>\n<value>Target Target</value></Operation>\n<Operation><xpath>Target</xpath></Operation></Patch>",
	})
	r := search(t, []Mod{mod}, Options{Pattern: "Target", Def: true, PerFile: 1, Offset: 100})
	if r.Shown != 0 || r.Matches != 5 || r.Kept != 2 {
		t.Fatal(r)
	}
	s := r.Mods[0].Summary
	if s.Roles["def"] != 1 || s.Roles["child"] != 1 || s.Roles["abstract"] != 1 || s.Roles["ref"] != 2 || s.Roles["patch"]+s.Roles["patch-value"] != 4 || s.PatchOperations != 2 {
		t.Fatalf("%+v", s)
	}
	if !reflect.DeepEqual(s.Defined, []string{"ThingDef/Target"}) || !reflect.DeepEqual(s.Abstracts, []string{"ThingDef@Target"}) || !reflect.DeepEqual(s.Children, []string{"ThingDef/Target"}) {
		t.Fatal(s)
	}
}
