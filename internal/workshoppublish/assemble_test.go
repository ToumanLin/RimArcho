package workshoppublish_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/workshoppublish"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"monitor/About/About.xml":                                                                  "<ModMetaData><name>Archo Monitor</name></ModMetaData>",
		"monitor/About/PublishedFileId.txt":                                                        "81234\n",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/About.xml":                      "<ModMetaData><name>Archo Monitor</name><packageId>test.monitor</packageId><supportedVersions><li>1.5</li><li>1.6</li><li>1.6</li><li>1.7</li></supportedVersions><description>A &amp; B</description><extra><name>Keep</name></extra></ModMetaData>",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/Preview.png":                    "synthetic preview",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/LoadFolders.xml":                      "<loadFolders/>",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/1.5/Assemblies/ArchoMonitor.dll":      "synthetic 1.5",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/1.6/Assemblies/ArchoMonitor.dll":      "synthetic 1.6",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/1.7/Assemblies/ArchoMonitor.dll":      "synthetic 1.7",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/Languages/English/Keyed/Settings.xml": "<LanguageData/>",
		"dist/archo-1.2.3-windows-amd64.zip":                                                       "synthetic zip payload",
		"dist/archo-1.2.3-windows-amd64.zip.sha256":                                                fmt.Sprintf("%x  archo-1.2.3-windows-amd64.zip\n", sha256.Sum256([]byte("synthetic zip payload"))),
		"README.md":                         "# synthetic readme",
		"README.zh-CN.md":                   "# synthetic readme zh",
		"UserGuide/user-guide.md":           "# synthetic guide",
		"UserGuide/user-guide.zh-CN.md":     "# synthetic guide zh",
		"skills/probe/SKILL.md":             "synthetic skill",
		"skills/probe/template/a.cs":        "class A {}",
		"skills/probe/template/bin/out.dll": "local build output",
		"skills/probe/template/obj/x.json":  "local build output",
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

func TestAssemble(t *testing.T) {
	root := fixture(t)
	item, err := workshoppublish.Assemble(root, "1.2.3", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(item.Folder) || item.ID != 81234 || item.Preview == "" || strings.Join(item.Tags, ",") != "Mod,1.5,1.6,1.7" {
		t.Fatalf("item: %+v", item)
	}
	for _, p := range []string{"About/PublishedFileId.txt", "About/Preview.png", "LoadFolders.xml", "1.5/Assemblies/ArchoMonitor.dll", "1.6/Assemblies/ArchoMonitor.dll", "1.7/Assemblies/ArchoMonitor.dll", "Languages/English/Keyed/Settings.xml", item.ZipName, item.ZipName + ".sha256",
		"README.md", "README.zh-CN.md", "UserGuide/user-guide.md", "UserGuide/user-guide.zh-CN.md", "skills/probe/SKILL.md", "skills/probe/template/a.cs"} {
		if _, err := os.Stat(filepath.Join(item.Folder, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
	// Workshop relocation cleanup keeps the item's own copies of release entries.
	top, err := os.ReadDir(item.Folder)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range top {
		if slices.Contains(selfupdate.Files, f.Name()) && !slices.Contains(selfupdate.ItemFiles, f.Name()) {
			t.Errorf("the item carries release entry %s, missing from selfupdate.ItemFiles", f.Name())
		}
	}
	for _, p := range []string{"skills/probe/template/bin", "skills/probe/template/obj"} {
		if _, err := os.Stat(filepath.Join(item.Folder, filepath.FromSlash(p))); !os.IsNotExist(err) {
			t.Fatalf("build output %s published", p)
		}
	}
	staged, err := os.ReadFile(filepath.Join(item.Folder, "About", "About.xml"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "dist", "archo-1.2.3-windows-amd64", "monitor", "ArchoMonitor", "About", "About.xml"))
	if err != nil || string(staged) != string(src) {
		t.Fatalf("About.xml must be copied unchanged: %s, %v", staged, err)
	}
	write(t, filepath.Join(item.Folder, "stale.zip"), []byte("old"))
	next, err := workshoppublish.Assemble(root, "1.2.3", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(next.Folder, "stale.zip")); !os.IsNotExist(err) {
		t.Fatal("stale file included")
	}
	write(t, item.IDFile, []byte("malformed default"))
	override, err := workshoppublish.Assemble(root, "1.2.3", 90000)
	if err != nil || override.ID != 90000 {
		t.Fatalf("explicit ID did not override source: %+v, %v", override, err)
	}
}

func TestAssembleValidation(t *testing.T) {
	for _, tc := range []struct{ name, file, content, want string }{
		{"checksum format", "dist/archo-1.2.3-windows-amd64.zip.sha256", "bad checksum", "no valid sha256"},
		{"checksum filename", "dist/archo-1.2.3-windows-amd64.zip.sha256", strings.Repeat("a", 64) + "  archo-9.9.9-windows-amd64.zip", "no valid sha256"},
		{"hash mismatch", "dist/archo-1.2.3-windows-amd64.zip", "changed", "does not match"},
		{"preview limit", "dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/Preview.png", strings.Repeat("x", 1<<20), "smaller than 1 MB"},
		{"bad ID", "monitor/About/PublishedFileId.txt", "not an id", "positive decimal"},
		{"no game versions", "dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/About.xml", "<ModMetaData><name>Archo Monitor</name></ModMetaData>", "no supported game versions"},
		{"bad game version", "dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/About.xml", "<ModMetaData><supportedVersions><li>1.6</li><li>v1.0.5</li></supportedVersions></ModMetaData>", "not a game version"},
		{"broken About.xml", "dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/About.xml", "<ModMetaData><supportedVersions>", "About.xml: "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixture(t)
			write(t, filepath.Join(root, filepath.FromSlash(tc.file)), []byte(tc.content))
			_, err := workshoppublish.Assemble(root, "1.2.3", 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if folders, _ := filepath.Glob(filepath.Join(root, "dist", "workshop-*")); len(folders) != 0 {
				t.Fatal("failed assembly left a partial staging folder")
			}
		})
	}
	for _, version := range []string{"1.2.3-rc1", "dev", "v1.2.3", "1.2.4", "../1.2.3"} {
		if _, err := workshoppublish.Assemble(fixture(t), version, 0); err == nil {
			t.Errorf("accepted %s", version)
		}
	}
}

func TestAssembleStandaloneBuildWithoutPreviewOrID(t *testing.T) {
	root := fixture(t)
	from := filepath.Join(root, "dist", "archo-1.2.3-windows-amd64", "monitor", "ArchoMonitor")
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
	item, err := workshoppublish.Assemble(root, "1.2.3", 0)
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != 0 || item.Preview != "" {
		t.Fatalf("item: %+v", item)
	}
}

func TestAssembleNeedsDocsAndSkills(t *testing.T) {
	for _, missing := range []string{"README.zh-CN.md", "UserGuide/user-guide.md", "skills/probe/SKILL.md"} {
		root := fixture(t)
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(missing))); err != nil {
			t.Fatal(err)
		}
		if _, err := workshoppublish.Assemble(root, "1.2.3", 0); err == nil {
			t.Errorf("assembled without %s", missing)
		}
		if folders, _ := filepath.Glob(filepath.Join(root, "dist", "workshop-*")); len(folders) != 0 {
			t.Fatal("failed assembly left a partial staging folder")
		}
	}
}

func TestAssemblePreviewPrefersRepositoryGIF(t *testing.T) {
	root := fixture(t)
	gif := filepath.Join(root, filepath.FromSlash(workshoppublish.PreviewAsset))
	write(t, gif, []byte("GIF89a synthetic"))
	item, err := workshoppublish.Assemble(root, "1.2.3", 0)
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
	item, err = workshoppublish.Assemble(root, "1.2.3", 0)
	if err != nil || item.Preview != filepath.Join(item.Folder, "About", "Preview.png") {
		t.Fatalf("without the GIF: %+v, %v", item, err)
	}
	write(t, gif, []byte(strings.Repeat("G", 1<<20)))
	if _, err := workshoppublish.Assemble(root, "1.2.3", 0); err == nil || !strings.Contains(err.Error(), "smaller than 1 MB") {
		t.Fatalf("1 MB GIF: %v", err)
	}
}
