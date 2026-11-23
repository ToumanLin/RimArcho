package workshoppublish_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/workshoppublish"
)

const built = "dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/"

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"monitor/About/About.xml":                      "<ModMetaData><name>Archo Monitor</name></ModMetaData>",
		"monitor/About/PublishedFileId.txt":            "81234\n",
		built + "About/About.xml":                      "<ModMetaData><name>Archo Monitor</name><packageId>test.monitor</packageId><supportedVersions><li>1.5</li><li>1.6</li><li>1.6</li><li>1.7</li></supportedVersions><description>A &amp; B</description><extra><name>Keep</name></extra></ModMetaData>",
		built + "About/Preview.png":                    "synthetic preview",
		built + "LoadFolders.xml":                      "<loadFolders/>",
		built + "LICENSE":                              "MIT",
		built + "1.5/Assemblies/ArchoMonitor.dll":      "synthetic 1.5",
		built + "1.6/Assemblies/ArchoMonitor.dll":      "synthetic 1.6",
		built + "1.7/Assemblies/ArchoMonitor.dll":      "synthetic 1.7",
		built + "Languages/English/Keyed/Settings.xml": "<LanguageData/>",
		"dist/archo-1.2.3-windows-amd64.zip":           "synthetic zip payload",
		"dist/archo-1.2.3-windows-amd64.zip.sha256":    "synthetic checksum",
		"dist/archo-1.2.3-windows-amd64/archo.exe":     "synthetic program",
		"README.md":                                        "# synthetic readme",
		"skills/probe/SKILL.md":                            "synthetic skill",
		"UserGuide/user-guide.md":                          "# synthetic guide",
		"dist/archo-1.2.3-linux-amd64.tar.gz":              "synthetic tarball",
		"dist/archo-1.2.3-linux-amd64.tar.gz.sha256":       "synthetic checksum",
		"dist/archo-1.2.3-windows-amd64/README.zh-CN.md":   "# synthetic readme zh",
		"dist/archo-1.2.3-windows-amd64/CHANGELOG.md":      "# synthetic changelog",
		"dist/archo-1.2.3-windows-amd64/RimArcho.exe":      "synthetic program",
		"dist/archo-1.2.3-windows-amd64/skills/a/SKILL.md": "synthetic skill",
	}
	for name, text := range files {
		write(t, filepath.Join(root, filepath.FromSlash(name)), []byte(text))
	}
	return root
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// files lists a folder's regular files as slash paths.
func files(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// The item is the built Mod and nothing else: no program, archive, checksum or reading
// files, whatever sits beside it in dist/ (17 §5.1).
func TestAssembleMonitorOnly(t *testing.T) {
	root := fixture(t)
	item, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(item.Folder) || item.ID != 81234 || item.Preview == "" || strings.Join(item.Tags, ",") != "Mod,1.5,1.6,1.7" {
		t.Fatalf("item: %+v", item)
	}
	want := "1.5/Assemblies/ArchoMonitor.dll,1.6/Assemblies/ArchoMonitor.dll,1.7/Assemblies/ArchoMonitor.dll,About/About.xml,About/Preview.png,About/PublishedFileId.txt,LICENSE,Languages/English/Keyed/Settings.xml,LoadFolders.xml"
	if got := strings.Join(files(t, item.Folder), ","); got != want {
		t.Fatalf("staged files:\n%s\nwant\n%s", got, want)
	}
	staged, err := os.ReadFile(filepath.Join(item.Folder, "About", "About.xml"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(built), "About", "About.xml"))
	if err != nil || string(staged) != string(src) {
		t.Fatalf("About.xml must be copied unchanged: %s, %v", staged, err)
	}
	write(t, filepath.Join(item.Folder, "stale.txt"), []byte("old"))
	next, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(next.Folder, "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("stale file included")
	}
	write(t, item.IDFile, []byte("malformed default"))
	override, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{ID: 90000})
	if err != nil || override.ID != 90000 {
		t.Fatalf("explicit ID did not override source: %+v, %v", override, err)
	}
	if id, _ := workshoppublish.ReadID(filepath.Join(override.Folder, "About", "PublishedFileId.txt")); id != 90000 {
		t.Fatalf("staged ID %d", id)
	}
}

// The retired program item is never published to: with it in PublishedFileId.txt the
// maintainer names the Monitor item or creates it (17 §5.1 bootstrap).
func TestAssembleRetiredItem(t *testing.T) {
	root := fixture(t)
	write(t, filepath.Join(root, "monitor", "About", "PublishedFileId.txt"), []byte("3835688284\n"))
	write(t, filepath.Join(root, filepath.FromSlash(built), "About", "PublishedFileId.txt"), []byte("3835688284\n"))
	if _, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{}); !errors.Is(err, workshoppublish.ErrRetiredItem) {
		t.Fatalf("retired ID in the file: %v", err)
	}
	if _, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{ID: workshoppublish.RetiredItem}); err == nil {
		t.Fatal("retired ID accepted as --item")
	}
	item, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{New: true})
	if err != nil || item.ID != 0 {
		t.Fatalf("new item: %+v, %v", item, err)
	}
	if _, err := os.Stat(filepath.Join(item.Folder, "About", "PublishedFileId.txt")); !os.IsNotExist(err) {
		t.Fatal("a new item's staging must not carry the retired ID")
	}
	item, err = workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{ID: 777})
	if err != nil || item.ID != 777 {
		t.Fatalf("--item over the retired file: %+v, %v", item, err)
	}
	if folders, _ := filepath.Glob(filepath.Join(root, "dist", "workshop-*")); len(folders) != 2 {
		t.Fatalf("refused assemblies left staging folders: %v", folders)
	}
}

// Anything that is not the Mod in a built Monitor stops the publish before Steam.
func TestAssembleRefusesProgramsAndArchives(t *testing.T) {
	for _, extra := range []string{"archo.exe", "Tools/RimArcho.EXE", "archo-1.2.3-windows-amd64.zip", "archo-1.2.3-windows-amd64.zip.sha256",
		"archo-1.2.3-linux-amd64.tar.gz", "Libs/helper.dll", "1.6/helper.dll", "1.6/Assemblies/sub/x.dll", "v1.6/Assemblies/x.dll"} {
		t.Run(extra, func(t *testing.T) {
			root := fixture(t)
			write(t, filepath.Join(root, filepath.FromSlash(built+extra)), []byte("x"))
			_, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{})
			if err == nil || !strings.Contains(err.Error(), "only the Archo Monitor Mod") || !strings.Contains(err.Error(), extra) {
				t.Fatalf("got %v", err)
			}
			if folders, _ := filepath.Glob(filepath.Join(root, "dist", "workshop-*")); len(folders) != 0 {
				t.Fatal("failed assembly left a partial staging folder")
			}
		})
	}
	// A second assembly in a game version's Assemblies folder is a Mod file.
	root := fixture(t)
	write(t, filepath.Join(root, filepath.FromSlash(built+"1.6/Assemblies/Helper.dll")), []byte("x"))
	if _, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{}); err != nil {
		t.Fatal(err)
	}
}

func TestAssembleValidation(t *testing.T) {
	for _, tc := range []struct{ name, file, content, want string }{
		{"preview limit", built + "About/Preview.png", strings.Repeat("x", 1<<20), "smaller than 1 MB"},
		{"bad ID", "monitor/About/PublishedFileId.txt", "not an id", "positive decimal"},
		{"no game versions", built + "About/About.xml", "<ModMetaData><name>Archo Monitor</name></ModMetaData>", "no supported game versions"},
		{"bad game version", built + "About/About.xml", "<ModMetaData><supportedVersions><li>1.6</li><li>v1.0.5</li></supportedVersions></ModMetaData>", "not a game version"},
		{"broken About.xml", built + "About/About.xml", "<ModMetaData><supportedVersions>", "About.xml: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			write(t, filepath.Join(root, filepath.FromSlash(tc.file)), []byte(tc.content))
			_, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if folders, _ := filepath.Glob(filepath.Join(root, "dist", "workshop-*")); len(folders) != 0 {
				t.Fatal("failed assembly left a partial staging folder")
			}
		})
	}
	for _, missing := range []string{"LoadFolders.xml", "1.7/Assemblies/ArchoMonitor.dll"} {
		root := fixture(t)
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(built+missing))); err != nil {
			t.Fatal(err)
		}
		if _, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{}); err == nil {
			t.Errorf("assembled without %s", missing)
		}
	}
	for _, version := range []string{"1.2.3-rc1", "dev", "v1.2.3", "../1.2.3"} {
		if _, err := workshoppublish.Assemble(fixture(t), version, workshoppublish.Options{}); err == nil {
			t.Errorf("accepted %s", version)
		}
	}
}

// A Monitor built on its own (monitor/build.ps1) publishes without a release folder.
func TestAssembleStandaloneBuildWithoutPreviewOrID(t *testing.T) {
	root := fixture(t)
	from := filepath.Join(root, filepath.FromSlash(built))
	to := filepath.Join(root, "monitor", "out", "ArchoMonitor")
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(to, "About", "Preview.png"), filepath.Join(root, "monitor", "About", "PublishedFileId.txt")} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	item, err := workshoppublish.Assemble(root, "1.2.4", workshoppublish.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != 0 || item.Preview != "" || item.Monitor != to {
		t.Fatalf("item: %+v", item)
	}
}

func TestAssemblePreviewPrefersRepositoryGIF(t *testing.T) {
	root := fixture(t)
	gif := filepath.Join(root, filepath.FromSlash(workshoppublish.PreviewAsset))
	write(t, gif, []byte("GIF89a synthetic"))
	item, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if item.Preview != gif {
		t.Fatalf("preview %s, want the repository GIF", item.Preview)
	}
	if _, err := os.Stat(filepath.Join(item.Folder, filepath.FromSlash(workshoppublish.PreviewAsset))); !os.IsNotExist(err) {
		t.Fatal("the page preview must stay outside the item folder")
	}
	if err := os.Remove(gif); err != nil {
		t.Fatal(err)
	}
	item, err = workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{})
	if err != nil || item.Preview != filepath.Join(item.Folder, "About", "Preview.png") {
		t.Fatalf("without the GIF: %+v, %v", item, err)
	}
	write(t, gif, []byte(strings.Repeat("G", 1<<20)))
	if _, err := workshoppublish.Assemble(root, "1.2.3", workshoppublish.Options{}); err == nil || !strings.Contains(err.Error(), "smaller than 1 MB") {
		t.Fatalf("1 MB GIF: %v", err)
	}
}
