package workshoppublish_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/workshoppublish"
)

// modFixture is an author's synthetic Mod folder with development files beside the Mod.
func modFixture(t *testing.T, extra map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"About/About.xml":             "<ModMetaData><name>Synthetic Beds</name><packageId>test.beds</packageId><supportedVersions><li>1.6</li><li>1.7</li></supportedVersions></ModMetaData>",
		"About/PublishedFileId.txt":   "4242\n",
		"About/preview.png":           "synthetic preview",
		"1.6/Assemblies/Beds.dll":     "synthetic 1.6",
		"1.7/Assemblies/Beds.dll":     "synthetic 1.7",
		"Defs/Beds.xml":               "<Defs/>",
		"Source/Beds.cs":              "// synthetic",
		"Source/obj/Release/Beds.dll": "build output",
		"Source/bin/Beds.dll":         "build output",
		".git/config":                 "synthetic",
		".gitignore":                  "obj/",
		"Defs/.vs/state":              "editor",
		"Backups/original/Beds.dll":   "old build",
		"Notes/todo.txt":              "private notes",
	}
	for name, text := range extra {
		files[name] = text
	}
	for name, text := range files {
		if text == "" {
			continue
		}
		write(t, filepath.Join(dir, filepath.FromSlash(name)), []byte(text))
	}
	return dir
}

func TestAssembleModStagesTheModOnly(t *testing.T) {
	dir := modFixture(t, nil)
	// A stray DLL outside a version's Assemblies is refused until it is excluded.
	if _, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{}); err == nil || !strings.Contains(err.Error(), "Backups/original/Beds.dll") || !strings.Contains(err.Error(), "--exclude") {
		t.Fatalf("stray DLL: %v", err)
	}
	item, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{Exclude: []string{"Backups/", `Notes\todo.txt`}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(item.Folder) })
	got := files(t, item.Folder)
	slices.Sort(got)
	want := []string{"1.6/Assemblies/Beds.dll", "1.7/Assemblies/Beds.dll", "About/About.xml", "About/PublishedFileId.txt", "About/preview.png", "Defs/Beds.xml", "Source/Beds.cs"}
	if !slices.Equal(got, want) {
		t.Fatalf("staged %q", got)
	}
	if item.ID != 4242 || item.Title != "Synthetic Beds" || !slices.Equal(item.Tags, []string{"Mod", "1.6", "1.7"}) || item.Files != 6 {
		t.Fatalf("item %+v", item)
	}
	if item.IDFile != filepath.Join(dir, "About", "PublishedFileId.txt") || item.Preview != filepath.Join(item.Folder, "About", "preview.png") {
		t.Fatalf("paths %+v", item)
	}
	slices.Sort(item.Excluded)
	if !slices.Equal(item.Excluded, []string{".git", ".gitignore", "Backups", "Defs/.vs", "Notes/todo.txt", "Source/bin", "Source/obj"}) {
		t.Fatalf("left out %q", item.Excluded)
	}
	if strings.HasPrefix(item.Folder, dir) {
		t.Fatal("staging inside the Mod folder")
	}
}

func TestAssembleModItemAndRefusals(t *testing.T) {
	ok := []string{"Backups", "Notes"}
	dir := modFixture(t, map[string]string{"About/PublishedFileId.txt": "3835688284"})
	if _, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{Exclude: ok}); err == nil {
		t.Fatal("published to the retired item")
	}
	item, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{ID: 99, Exclude: ok})
	if err != nil || item.ID != 99 {
		t.Fatalf("--item: %+v %v", item, err)
	}
	os.RemoveAll(item.Folder)
	b, _ := os.ReadFile(filepath.Join(dir, "About", "PublishedFileId.txt"))
	if string(b) != "3835688284" {
		t.Fatal("staging changed the Mod's ID file")
	}

	dir = modFixture(t, nil)
	os.Remove(filepath.Join(dir, "About", "PublishedFileId.txt"))
	if _, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{Exclude: ok}); err == nil || !strings.Contains(err.Error(), "--new-item") {
		t.Fatalf("no ID file: %v", err)
	}
	item, err = workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{New: true, Exclude: ok})
	if err != nil || item.ID != 0 {
		t.Fatalf("--new-item: %+v %v", item, err)
	}
	if _, err := os.Stat(filepath.Join(item.Folder, "About", "PublishedFileId.txt")); !os.IsNotExist(err) {
		t.Fatal("a new item staged an ID file")
	}
	os.RemoveAll(item.Folder)

	for _, bad := range []string{"..", "../x", "."} {
		if _, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{New: true, Exclude: []string{bad}}); err == nil {
			t.Fatalf("--exclude %q accepted", bad)
		}
	}
	if _, err := workshoppublish.AssembleMod(t.TempDir(), workshoppublish.ModOptions{New: true}); err == nil || !strings.Contains(err.Error(), "About.xml") {
		t.Fatalf("not a Mod: %v", err)
	}
	dir = modFixture(t, map[string]string{"About/About.xml": "<ModMetaData><name>No Versions</name></ModMetaData>"})
	if _, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{Exclude: ok}); err == nil || !strings.Contains(err.Error(), "supported game versions") {
		t.Fatalf("no versions: %v", err)
	}
	dir = modFixture(t, map[string]string{"Tools/run.exe": "program"})
	if _, err := workshoppublish.AssembleMod(dir, workshoppublish.ModOptions{Exclude: ok}); err == nil || !strings.Contains(err.Error(), "Tools/run.exe") {
		t.Fatalf("program: %v", err)
	}
}
