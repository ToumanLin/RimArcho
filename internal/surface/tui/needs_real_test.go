package tui

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ToumanLin/RimArcho/internal/app"
)

// What must never appear on the 需要处理 screen (06 §7): problem codes, #refs and the other
// reference forms of the CLI.
var leakRe = regexp.MustCompile(`\b(?:dep|order|official|incompat|pkg|patch|version|log|harmony|def|mod|entry)\.[a-z][a-z-]+\b|#[a-z0-9]{4,5}\b|L#[a-z0-9]+|@\{|\bpkg:|\bp:[0-9a-f]{6}\b|\blog:E?\d|\bscan:\d|\bq:\d|\br:\d`)

// T8 (06 §11) on a copy of a real data directory: every item of 需要处理 reads as one
// sentence in plain words, has keys, and each key does something (opens a view or a
// preview, or acts and says so); no code or #ref is on the screen. The data directory is
// copied again into a temp dir first, so the keys' edits land nowhere that is kept.
//
//	RIMARCHO_REAL_COPY=<copy of %APPDATA%\RimArcho> go test ./internal/surface/tui -run T8 -v
func TestT8NeedsReal(t *testing.T) {
	src := os.Getenv("RIMARCHO_REAL_COPY")
	if src == "" {
		t.Skip("set RIMARCHO_REAL_COPY to a copy of a real data directory")
	}
	home := t.TempDir()
	for _, f := range []string{"archo.db", "archo.db-wal", "config.toml"} {
		copyFile(t, filepath.Join(src, f), filepath.Join(home, f))
	}
	a, err := app.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Launcher = func(string, []string, string) error { t.Fatal("launch attempted"); return nil }
	a.AnalyzeRunner = app.InProcessAnalyze(a) // never the child: that would be this test binary
	opened := ""
	m := New(Options{App: a, OpenURL: func(u string) error { opened = u; return nil },
		Scan:    func(string) (string, error) { t.Fatal("scan"); return "", nil },
		Analyze: func(string, func(string)) error { _, err := a.Analyze(nil); return err }}).(*model)
	m.Init()
	m.update(tea.WindowSizeMsg{Width: 160, Height: 46})
	m.app.LazyLog, m.app.LogCacheOnly = false, false
	m.tab = tabProblems
	m.loadProblems()
	defer SetLang("en")
	for _, lang := range []string{"en", "zh"} {
		SetLang(lang)
		m.loadProblems()
		if len(m.prob.items) == 0 {
			t.Fatal("no items on real data")
		}
		if got, want := m.needCount(), countedNeeds(m); got != want {
			t.Fatalf("top bar %d, counted items %d", got, want)
		}
		t.Logf("[%s] %d items, %d counted", lang, len(m.prob.items), m.needCount())
		for i, it := range m.prob.items {
			t.Logf("  %s %s  [%s]", map[bool]string{true: "*", false: " "}[it.counted], it.text, keyList(it))
			if it.text == "" || strings.ContainsAny(it.text, "\n") || strings.Contains(strings.TrimSuffix(it.text, "."), ". ") ||
				strings.Count(strings.TrimSuffix(it.text, "。"), "。") > 0 {
				t.Errorf("item %d is not one sentence: %q", i, it.text)
			}
			if leakRe.MatchString(it.text) {
				t.Errorf("item %d shows a code or a ref: %q", i, it.text)
			}
			if len(it.keys) == 0 {
				t.Errorf("item %d has no key: %q", i, it.text)
			}
			for _, k := range it.keys {
				if leakRe.MatchString(k.label) {
					t.Errorf("key label shows a code or a ref: %q", k.label)
				}
			}
			// the whole screen with this item selected
			m.prob.cursor, m.prob.off = i, 0
			for _, l := range m.needLines(4000) {
				if p := ansi.Strip(l); leakRe.MatchString(p) {
					t.Errorf("screen line shows a code or a ref: %q", p)
				}
			}
		}
	}
	SetLang("en")
	m.loadProblems()
	// One key fixes every order violation: they are one item, counted once (06 §7.2).
	folds := 0
	for i, it := range m.prob.items {
		if strings.Contains(it.text, "should load after") {
			t.Errorf("order violation not folded: %q", it.text)
		}
		if !it.fold {
			continue
		}
		folds++
		if !strings.HasSuffix(it.text, "mods are out of order") || len(it.parts) < 2 || keyList(it) != "S sort · enter show which" {
			t.Errorf("fold %q: %d parts, keys %s", it.text, len(it.parts), keyList(it))
		}
		m.prob.cursor = i
		m.needsKey("enter")
		shown := strings.Join(func() []string {
			var ls []string
			for _, l := range m.needLines(4000) {
				ls = append(ls, ansi.Strip(l))
			}
			return ls
		}(), "\n")
		for _, part := range it.parts {
			if !strings.Contains(shown, part) {
				t.Errorf("enter does not show %q", part)
			}
		}
		m.needsKey("enter")
	}
	if folds != 1 {
		t.Errorf("%d folded items, want 1", folds)
	}
	// Every key does something. Edits only change this temp copy; nothing is saved.
	texts := needItems(m)
	ran := 0
	for _, text := range texts {
		for ki := 0; ; ki++ {
			m.loadProblems()
			i := findNeed(m, text)
			if i < 0 || ki >= len(m.prob.items[i].keys) {
				break // resolved by an earlier key, or all keys tried
			}
			k := m.prob.items[i].keys[ki]
			m.panel, m.insp, m.status, m.sayKind, opened = nil, nil, "", sayNone, ""
			m.prob.cursor = i
			key := k.key
			if key == "space" {
				key = " "
			}
			wasOpen := m.prob.open[text]
			m.needsKey(key)
			ran++
			did := m.panel != nil || m.insp != nil || m.sayKind == sayOK || opened != "" || m.prob.open[text] != wasOpen
			if !did || m.sayKind == sayFail {
				t.Errorf("%q: key %s (%s) did nothing: status %q", text, k.key, k.label, m.status)
			}
			if m.panel != nil && m.panel.title == T("order requirements that contradict each other") {
				for _, l := range m.panel.lines {
					t.Logf("    cycle: %s", l)
					if leakRe.MatchString(l) {
						t.Errorf("cycle view shows a code or a ref: %q", l)
					}
				}
			}
			if m.panel != nil && m.panel.choices != nil {
				// a chooser: pick the first choice, as the player would
				m.panelKey("enter")
				if m.sayKind == sayFail {
					t.Errorf("%q: choice failed: %q", text, m.status)
				}
			}
			m.panel, m.insp = nil, nil
		}
	}
	if ran < len(texts) {
		t.Fatalf("only %d keys ran for %d items", ran, len(texts))
	}
	t.Logf("%d keys ran", ran)
}

func keyList(it need) string {
	var ks []string
	for _, k := range it.keys {
		ks = append(ks, k.key+" "+k.label)
	}
	return strings.Join(ks, " · ")
}

func copyFile(t *testing.T, from, to string) {
	in, err := os.Open(from)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}
