package app

import (
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Ignores are keyed by findingKey, so a relative path must pass through unchanged.
func TestFindingKeyKeepsRelativePaths(t *testing.T) {
	if k := "XML error in Defs/ThingDefs/Foo.xml"; findingKey(k) != k {
		t.Errorf("findingKey(%q) = %q", k, findingKey(k))
	}
	if got := findingKey(`load D:\Steam\steamapps\workshop\content\294100\1\Defs\a.xml`); got != "load 1/defs/a.xml" {
		t.Errorf("drive path: %q", got)
	}
}

func TestFindingKeyAudioPaths(t *testing.T) {
	keyFor := func(path string) string {
		e := &logs.Entry{Message: "Exception loading UnityEngine.AudioClip from file.", Extra: []string{"absFilePath: " + path}}
		key, _, _ := logs.ClusterKey(e)
		return findingKey(key)
	}
	const tail = "/steamapps/workshop/content/294100/1234567890/1.6/Sounds/x.mp3"
	want := keyFor("/synthetic/Steam" + tail)
	if !strings.HasSuffix(want, "|absFilePath: 1.6/sounds/x.mp3") {
		t.Fatalf("audio path was not shortened: %q", want)
	}
	for _, root := range []string{"/home/synthetic user/.local/share/Steam", "/mnt/Synthetic Games/SteamLibrary", "D:/Synthetic Games/SteamLibrary", `D:\Synthetic Games\SteamLibrary`} {
		path := root + tail
		if strings.Contains(root, `\`) {
			path = strings.ReplaceAll(path, "/", `\`)
		}
		if got := keyFor(path); got != want {
			t.Errorf("%q: key %q, want %q", path, got, want)
		}
	}
	// Previously stable drive-path keys keep their exact text (and ignore refs).
	for _, ext := range []string{"xml", "dll", "png", "txt"} {
		if got := findingKey(`file: D:\Synthetic Games\Mods\A\Defs\x.` + ext); got != "file: a/defs/x."+ext {
			t.Errorf("existing Windows key changed: %q", got)
		}
	}
}
