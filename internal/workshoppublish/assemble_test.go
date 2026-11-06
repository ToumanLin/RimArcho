package workshoppublish_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/workshoppublish"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"monitor/About/About.xml":                                                                  "<ModMetaData><name>Archo Monitor</name></ModMetaData>",
		"monitor/About/PublishedFileId.txt":                                                        "81234\n",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/About.xml":                      "<ModMetaData><name>Archo Monitor</name><packageId>test.monitor</packageId><description>A &amp; B</description><extra><name>Keep</name></extra></ModMetaData>",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/Preview.png":                    "synthetic preview",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/LoadFolders.xml":                      "<loadFolders/>",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/1.5/Assemblies/ArchoMonitor.dll":      "synthetic 1.5",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/1.6/Assemblies/ArchoMonitor.dll":      "synthetic 1.6",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/1.7/Assemblies/ArchoMonitor.dll":      "synthetic 1.7",
		"dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/Languages/English/Keyed/Settings.xml": "<LanguageData/>",
		"dist/archo-1.2.3-windows-amd64.zip":                                                       "synthetic zip payload",
		"dist/archo-1.2.3-windows-amd64.zip.sha256":                                                fmt.Sprintf("%x  archo-1.2.3-windows-amd64.zip\n", sha256.Sum256([]byte("synthetic zip payload"))),
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
	if !filepath.IsAbs(item.Folder) || item.ID != 81234 || item.Preview == "" {
		t.Fatalf("item: %+v", item)
	}
	for _, p := range []string{"About/PublishedFileId.txt", "About/Preview.png", "LoadFolders.xml", "1.5/Assemblies/ArchoMonitor.dll", "1.6/Assemblies/ArchoMonitor.dll", "1.7/Assemblies/ArchoMonitor.dll", "Languages/English/Keyed/Settings.xml", item.ZipName, item.ZipName + ".sha256"} {
		if _, err := os.Stat(filepath.Join(item.Folder, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(item.Folder, "About", "About.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<name>RimArcho</name>") || !strings.Contains(string(b), "<packageId>test.monitor</packageId>") || !strings.Contains(string(b), "A &amp; B") {
		t.Fatalf("metadata: %s", b)
	}
	if !strings.Contains(string(b), "<extra><name>Keep</name></extra>") {
		t.Fatal("changed nested metadata name")
	}
	b, err = os.ReadFile(filepath.Join(root, "monitor", "About", "About.xml"))
	if err != nil || strings.Contains(string(b), "<name>RimArcho</name>") {
		t.Fatal("source metadata changed", err)
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
		{"bad metadata", "dist/archo-1.2.3-windows-amd64/monitor/ArchoMonitor/About/About.xml", "<ModMetaData>", "XML"},
		{"bad ID", "monitor/About/PublishedFileId.txt", "not an id", "positive decimal"},
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
