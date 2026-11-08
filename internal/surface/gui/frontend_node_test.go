package gui

import (
	"os/exec"
	"testing"
)

// The frontend behavior suites under testdata (dom.mjs is their shared helper).
// One node process runs them all; node --test still isolates each file.
var frontendNodeSuites = []string{
	"testdata/accent.mjs",
	"testdata/archo.mjs",
	"testdata/badgepop.mjs",
	"testdata/command-panel.mjs",
	"testdata/credit.mjs",
	"testdata/datatables.mjs",
	"testdata/deps.mjs",
	"testdata/detail-follow.mjs",
	"testdata/editorfilter.mjs",
	"testdata/guide.mjs",
	"testdata/home-memory.mjs",
	"testdata/firstrun.mjs",
	"testdata/packs.mjs",
	"testdata/paths.mjs",
	"testdata/planmanage.mjs",
	"testdata/separators.mjs",
	"testdata/status-motion.mjs",
	"testdata/subscribe.mjs",
	"testdata/subscriptions.mjs",
	"testdata/textures.mjs",
	"testdata/window-chrome.mjs",
}

func TestFrontendNodeSuites(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for the frontend behavior tests")
	}
	args := append([]string{"--experimental-vm-modules", "--test"}, frontendNodeSuites...)
	if out, err := exec.Command(node, args...).CombinedOutput(); err != nil {
		t.Fatalf("frontend suites: %v\n%s", err, out)
	}
}
