//go:build windows

package selfupdate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestWorkshopCopyCleanupKeepsItem(t *testing.T) {
	e := testkit.New(t)
	from := filepath.Join(e.Workshop, "987654321")
	target := filepath.Join(e.Root, "installed")
	release := []string{"archo.exe", "RimArcho.exe", "CHANGELOG.md", "monitor/ArchoMonitor/About/About.xml"}
	keep := []string{"About/About.xml", "About/PublishedFileId.txt", "LoadFolders.xml", "1.6/Assemblies/Monitor.dll", "Languages/English/Keyed/Monitor.xml", "release.zip", "release.zip.sha256", "PublishedFileId"}
	for _, name := range release {
		e.WriteFile(filepath.Join(from, filepath.FromSlash(name)), "release "+name)
	}
	for _, name := range keep {
		e.WriteFile(filepath.Join(from, filepath.FromSlash(name)), "item "+name)
	}
	e.WriteFile(filepath.Join(target, "RimArcho.exe"), "old")
	e.WriteFile(filepath.Join(target, "personal.txt"), "keep")
	swap, err := selfupdate.CopyRelease(from, target)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range release {
		b, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if err != nil || string(b) != "release "+name {
			t.Fatalf("copy %s: %q %v", name, b, err)
		}
	}
	if err := selfupdate.CleanWorkshopRelease(from); err != nil {
		t.Fatal(err)
	}
	for _, name := range keep {
		b, err := os.ReadFile(filepath.Join(from, filepath.FromSlash(name)))
		if err != nil || string(b) != "item "+name {
			t.Fatalf("item changed %s: %q %v", name, b, err)
		}
	}
	for _, name := range release {
		if _, err := os.Stat(filepath.Join(from, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("release remains: %s", name)
		}
	}
	if err := swap.Rollback(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "RimArcho.exe")); err != nil || string(b) != "old" {
		t.Fatalf("rollback: %q %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(target, "personal.txt")); err != nil || string(b) != "keep" {
		t.Fatal("unrelated destination changed")
	}
	if err := selfupdate.CleanWorkshopRelease(target); err == nil {
		t.Fatal("cleanup outside Workshop accepted")
	}
}

// The published item ships README.md, README.zh-CN.md, UserGuide/, skills/ and LICENSE
// beside the zip (17 §5.1 Upload). A release extracted into the item root overwrites
// them with the same bytes; cleanup removes only what the item does not carry.
func TestWorkshopCleanupKeepsItemDocs(t *testing.T) {
	e := testkit.New(t)
	item := filepath.Join(e.Workshop, "3835688284")
	own := []string{"About/About.xml", "LoadFolders.xml", "1.6/Assemblies/ArchoMonitor.dll", "Languages/English/Keyed/Monitor.xml",
		"archo-1.0.6-windows-amd64.zip", "archo-1.0.6-windows-amd64.zip.sha256", "LICENSE",
		"README.md", "README.zh-CN.md", "UserGuide/user-guide.md", "UserGuide/user-guide.zh-CN.md", "skills/rimworld-probe/SKILL.md"}
	extracted := []string{"archo.exe", "RimArcho.exe", "CHANGELOG.md", "monitor/ArchoMonitor/About/About.xml"}
	for _, name := range own {
		e.WriteFile(filepath.Join(item, filepath.FromSlash(name)), "item "+name)
	}
	for _, name := range extracted {
		e.WriteFile(filepath.Join(item, filepath.FromSlash(name)), "release "+name)
	}
	if err := selfupdate.CleanWorkshopRelease(item); err != nil {
		t.Fatal(err)
	}
	for _, name := range own {
		if b, err := os.ReadFile(filepath.Join(item, filepath.FromSlash(name))); err != nil || string(b) != "item "+name {
			t.Errorf("item file changed %s: %q %v", name, b, err)
		}
	}
	for _, name := range append(extracted, "monitor") {
		if _, err := os.Stat(filepath.Join(item, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Errorf("release entry remains: %s", name)
		}
	}

	// Extracted into a folder of its own below the item: every release entry goes.
	child := filepath.Join(item, "archo-1.0.6-windows-amd64")
	for _, name := range append(extracted, "README.md", "README.zh-CN.md", "LICENSE", "skills/rimworld-probe/SKILL.md") {
		e.WriteFile(filepath.Join(child, filepath.FromSlash(name)), "release "+name)
	}
	if err := selfupdate.CleanWorkshopRelease(child); err != nil {
		t.Fatal(err)
	}
	if left, err := os.ReadDir(child); err != nil || len(left) != 0 {
		t.Fatalf("child folder keeps %v %v", left, err)
	}
	for _, name := range own {
		if b, err := os.ReadFile(filepath.Join(item, filepath.FromSlash(name))); err != nil || string(b) != "item "+name {
			t.Errorf("item file changed by the child cleanup %s: %q %v", name, b, err)
		}
	}
}
