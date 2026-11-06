package selfupdate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/selfupdate"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestInWorkshopFolder(t *testing.T) {
	for _, tc := range []struct{ path, item string }{
		{`D:/SteamLibrary/steamapps/workshop/content/294100/123`, `D:/SteamLibrary/steamapps/workshop/content/294100/123`},
		{`E:/Games/SteamApps/Workshop/Content/294100/000123/release/`, `E:/Games/SteamApps/Workshop/Content/294100/000123`},
		{`C:\Program Files (x86)\Steam\steamapps\workshop\content\294100\456\`, `C:/Program Files (x86)/Steam/steamapps/workshop/content/294100/456`},
		{`//server/library/steamapps/workshop/content/294100/9/app`, `//server/library/steamapps/workshop/content/294100/9`},
		{`D:/steamapps/workshop/content/294100/123evil`, ""},
		{`D:/steamapps/workshop/content/294100`, ""},
		{`D:/steamapps/workshop/content/294100/-1`, ""},
		{`D:/steamapps/workshop/content/123/456`, ""},
		{`D:/notsteamapps/workshop/content/294100/123`, ""},
		{`D:/steamapps/workshop/content/294100/123/../../elsewhere`, ""},
	} {
		item, ok := selfupdate.InWorkshopFolder(tc.path)
		if ok != (tc.item != "") || !strings.EqualFold(filepath.Clean(item), filepath.Clean(filepath.FromSlash(tc.item))) {
			t.Errorf("%q = %q,%v; want %q", tc.path, item, ok, tc.item)
		}
	}
}

func TestWorkshopCopyCleanupKeepsItem(t *testing.T) {
	e := testkit.New(t)
	from := filepath.Join(e.Workshop, "987654321")
	target := filepath.Join(e.Root, "installed")
	release := []string{"archo.exe", "RimArcho.exe", "README.md", "monitor/ArchoMonitor/About/About.xml", "skills/example/SKILL.md"}
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
