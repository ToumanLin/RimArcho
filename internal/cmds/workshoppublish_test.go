package cmds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

func TestWorkshopPublishArguments(t *testing.T) {
	for _, tc := range []struct {
		args          []string
		version       string
		id            uint64
		visibility    uint32
		hasVisibility bool
	}{
		{[]string{"--version", "1.2.3", "--dry-run"}, "1.2.3", 0, 0, false},
		{[]string{"--version=1.2.3", "--item", "81234", "--note", "Release 1.2.3", "--visibility", "unlisted"}, "1.2.3", 81234, 3, true},
		{[]string{"--version", "1.2.3", "--visibility", "public"}, "1.2.3", 0, 0, true},
		{[]string{"--version", "1.2.3", "--visibility", "friends"}, "1.2.3", 0, 1, true},
		{[]string{"--version", "1.2.3", "--visibility", "private"}, "1.2.3", 0, 2, true},
	} {
		a, err := parse(workshopPublishCommand, tc.args)
		if err != nil {
			t.Fatal(err)
		}
		r, err := publishOptions(a)
		if err != nil {
			t.Fatal(err)
		}
		if r.Version != tc.version || r.ID != tc.id || (r.Visibility != nil) != tc.hasVisibility || tc.hasVisibility && *r.Visibility != tc.visibility {
			t.Fatalf("options: %+v", r)
		}
	}
	for _, args := range [][]string{
		{"--version", "1.2.3-rc1"}, {"--version", "dev"}, {"--version", ""},
		{"--version", "1.2.3", "--item", "0"}, {"--version", "1.2.3", "--item", "1junk"},
		{"--version", "1.2.3", "--item", "18446744073709551616"},
		{"--version", "1.2.3", "--visibility", "hidden"},
		{"--version", "1.2.3", "--note", "bad\x00note"},
		{"--version", "1.2.3", "--note", strings.Repeat("x", 8001)},
		{"--version"}, {"--item"}, {"--unexpected"}, {"folder"}, {"--dry-run=true"},
	} {
		a, err := parse(workshopPublishCommand, args)
		if err == nil {
			_, err = publishOptions(a)
		}
		if err == nil {
			t.Fatalf("accepted %q", args)
		}
		if !WorkshopPublishOffline(append([]string{"workshop", "publish"}, args...)) {
			t.Fatalf("bad arguments would open a player home: %q", args)
		}
	}
	old := buildinfo.Version
	buildinfo.Version = "2.3.4"
	t.Cleanup(func() { buildinfo.Version = old })
	a, _ := parse(workshopPublishCommand, nil)
	r, err := publishOptions(a)
	if err != nil || r.Version != "2.3.4" {
		t.Fatalf("default version: %+v, %v", r, err)
	}
	if WorkshopPublishOffline([]string{"workshop", "publish"}) {
		t.Fatal("real publish bypasses configured environment")
	}
	if !WorkshopPublishOffline([]string{"workshop", "publish", "--help"}) {
		t.Fatal("help opens player home")
	}
}

func TestWorkshopPublishDescriptionAndPreview(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, size int, fill string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(strings.Repeat(fill, size)), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	en := write("post.txt", 1, "[h3]RimArcho[/h3]\n")
	zh := write("post.zh-CN.txt", 1, "\uFEFF[h3]\u542f\u52a8\u524d\u4f53\u68c0[/h3]\n")
	gif := write("icon.gif", 900<<10, "G")
	a, err := parse(workshopPublishCommand, []string{"--version", "1.2.3", "--description", en, "--description-zh", zh, "--preview", gif})
	if err != nil {
		t.Fatal(err)
	}
	r, err := publishOptions(a)
	if err != nil {
		t.Fatal(err)
	}
	if r.Description != "[h3]RimArcho[/h3]" || len(r.Translations) != 1 || r.Translations[0] != (steamworks.PublishTranslation{Language: "schinese", Description: "[h3]\u542f\u52a8\u524d\u4f53\u68c0[/h3]"}) || r.Preview != gif {
		t.Fatalf("options: %+v", r)
	}
	big := write("big.txt", 8001, "x")
	empty := write("empty.txt", 0, "")
	bmp := write("icon.bmp", 10, "B")
	huge := write("huge.gif", 1<<20, "G")
	for _, args := range [][]string{
		{"--description", big}, {"--description-zh", big}, {"--description", empty},
		{"--description", filepath.Join(dir, "missing.txt")}, {"--description"},
		{"--preview", bmp}, {"--preview", huge}, {"--preview", filepath.Join(dir, "missing.png")},
	} {
		a, err := parse(workshopPublishCommand, append([]string{"--version", "1.2.3"}, args...))
		if err == nil {
			_, err = publishOptions(a)
		}
		if err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
}
