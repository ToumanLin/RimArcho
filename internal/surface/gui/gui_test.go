package gui

import (
	"io/fs"
	"maps"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

var textRe = regexp.MustCompile(`\bTf?\("((?:[^"\\]|\\.)*)"`)

// Every string the page translates has a Chinese entry (16 §7; the page has no table of
// its own: it gets view.Table and view.ExtraText).
func TestPageTextTranslated(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	table := maps.Clone(view.Table())
	maps.Copy(table, view.ExtraText())
	var missing []string
	fs.WalkDir(assets, "assets/js", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		b, _ := fs.ReadFile(assets, p)
		for _, m := range textRe.FindAllStringSubmatch(string(b), -1) {
			s := strings.ReplaceAll(m[1], `\"`, `"`)
			if _, ok := table[s]; !ok {
				missing = append(missing, s)
			}
		}
		return nil
	})
	sort.Strings(missing)
	missing = compactStrings(missing)
	if len(missing) > 0 {
		t.Errorf("%d page strings have no Chinese text:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func compactStrings(s []string) []string {
	var out []string
	for i, x := range s {
		if i == 0 || x != s[i-1] {
			out = append(out, x)
		}
	}
	return out
}

// ui.gpu off adds the software-drawing switches; both keep the network service in the
// browser process and one renderer (17 §7.3).
func TestBrowserArgs(t *testing.T) {
	t.Parallel()
	on, off := strings.Join(BrowserArgs(true), " "), strings.Join(BrowserArgs(false), " ")
	for _, s := range []string{on, off} {
		if !strings.Contains(s, "NetworkServiceInProcess2") || !strings.Contains(s, "--renderer-process-limit=1") {
			t.Fatalf("missing the shared switches: %s", s)
		}
	}
	if strings.Contains(on, "--disable-gpu") || !strings.Contains(off, "--disable-gpu --disable-gpu-compositing --in-process-gpu") {
		t.Fatalf("gpu switches: on %q, off %q", on, off)
	}
}

// The same embedded files used by TestPageTextTranslated also contain untranslated
// separators and template literals. Inspect the whole source, excluding comments.
func TestWording(t *testing.T) {
	t.Parallel()
	commentsAndStrings := regexp.MustCompile("(?s)//[^\n]*|/\\*.*?\\*/|\"(?:[^\"\\\\]|\\\\.)*\"|'(?:[^'\\\\]|\\\\.)*'|`(?:[^`\\\\]|\\\\.)*`")
	err := fs.WalkDir(assets, "assets/js", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		b, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for _, token := range commentsAndStrings.FindAllString(string(b), -1) {
			if strings.HasPrefix(token, "//") || strings.HasPrefix(token, "/*") {
				continue
			}
			text := strings.ReplaceAll(token, `\u00b7`, string(rune(0xB7)))
			for _, p := range view.WordingTextProblems(path, text) {
				t.Error(p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
