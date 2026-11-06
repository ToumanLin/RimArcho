package cmds

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/buildinfo"
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
