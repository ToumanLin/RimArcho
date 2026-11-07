package l10ndata

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Every permanent exception has a specific purpose; test data exceptions name individual files.
var sourceLanguageFiles = map[string]string{
	"docs/glossary.md":                                       "The bilingual terminology reference.",
	"README.zh-CN.md":                                        "The Chinese README.",
	"docs/user-guide.zh-CN.md":                               "The Chinese player user guide.",
	"docs/highlights.zh-CN.md":                               "The Chinese release highlights for players.",
	"ToDo.md":                                                "The maintainer's working list; notes are written in whichever language is quicker.",
	"monitor/About/About.xml":                                "RimWorld does not localize About.xml descriptions; keep the bilingual description.",
	"internal/app/filelists_golden_test.go":                  "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/app/packname_test.go":                          "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/app/parentproblems_test.go":                    "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/app/subscribeworkshoprun_test.go":              "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/app/uninstall_deps_test.go":                    "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/cmds/config_test.go":                           "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/logs/richtext_test.go":                         "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/menubg/cache_test.go":                          "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/modsearch/modsearch_test.go":                   "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/scan/utf16_test.go":                            "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/store/content_bench_test.go":                   "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/store/content_test.go":                         "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/store/derived_scan_test.go":                    "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/store/intkey_research_test.go":                 "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/store/filelist_test.go":                        "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/gui/credit_test.go":                    "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/gui/exchange_test.go":                  "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/gui/linkhealth_test.go":                "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/gui/style_rules_test.go":               "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/gui/tasks_test.go":                     "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/gui/testdata/editorfilter.mjs":         "Exercises localized labels and CJK path or terminal rendering.",
	"internal/surface/gui/testdata/paths.mjs":                "Exercises localized labels and CJK path or terminal rendering.",
	"internal/surface/gui/useinstalled_test.go":              "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/detail_test.go":                    "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/latency_test.go":                   "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/live_test.go":                      "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/needs_real_test.go":                "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/palette_test.go":                   "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/selection_test.go":                 "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/testdata/TestSelectionRows.golden": "Exercises localized labels and CJK path or terminal rendering.",
	"internal/surface/tui/tui_test.go":                       "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/v3_test.go":                        "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/tui/widths_test.go":                    "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/upgradewin/upgradewin_test.go":         "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/surface/upgradewin/upgradewin_windows_test.go": "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/textenc/textenc_test.go":                       "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"internal/view/accent_test.go":                           "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/accounts_test.go":                         "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/analyzeprogress_test.go":                  "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/autoupdate_test.go":                       "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/cliline_test.go":                          "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/bar_test.go":                              "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/companion_internal_test.go":               "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/copycaption_test.go":                      "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/copynote_test.go":                         "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/deps_test.go":                             "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/editorfilter_test.go":                     "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/lastrun_test.go":                          "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/library_test.go":                          "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/loadcost_test.go":                         "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/maintenancetext_test.go":                  "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/memory_test.go":                           "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/monitorlog_test.go":                       "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/needlinks_bench_test.go":                  "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/needlinks_test.go":                        "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/ops_test.go":                              "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/packpage_test.go":                         "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/saves_play_test.go":                       "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/steamtext_test.go":                        "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/subscription_test.go":                     "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/subscriptions_test.go":                    "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/taskagain_test.go":                        "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/taskdownload_test.go":                     "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/tasksubscription_test.go":                 "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/uninstall_confirmation_test.go":           "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/versiondiff_test.go":                      "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"internal/view/versionmute_test.go":                      "Asserts Chinese interface text or searches Chinese tags and mod names.",
	"scripts/vm/choices.test.mjs":                            "Asserts localized interface text and CJK control behavior.",
	"scripts/vm/needtext.test.mjs":                           "Asserts localized interface text and CJK control behavior.",
	"spikes/guidev/pagecheck.js":                             "Asserts localized interface text and CJK control behavior.",
	"spikes/hugslogs/parse_test.go":                          "Exercises Chinese interface output, legacy Chinese data, or CJK names and paths.",
	"spikes/tuilist/bench.go":                                "Measures CJK search, clipping, alignment, and terminal glyph widths.",
	"spikes/tuilist/model.go":                                "Measures CJK search, clipping, alignment, and terminal glyph widths.",
	"spikes/tuiv2-v0/glyphgrid/main.go":                      "Measures CJK search, clipping, alignment, and terminal glyph widths.",
	"testdata/companion/RimCrow_Realtime.log":                "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/companion/real17/Logs/Player.log":              "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/golden/basics.txt":                             "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/golden/loadout.txt":                            "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/golden/mcp_manage.txt":                         "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/mods/versioned/About/About.xml":                "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/steamcmd/app_info_print_294100.txt":            "Preserves Chinese names, encoded input, or localized output in a fixture.",
	"testdata/store/filelists-scenario.txt":                  "Preserves Chinese names, encoded input, or localized output in a fixture.",
}

var sourceLanguageDirectories = map[string]string{
	"internal/l10ndata/locale/": "Embedded language resources, compatibility text, and the existing tag preset.",
	"monitor/Languages/":        "RimWorld Keyed language resources.",
}

func sourceLanguageExempt(path string) bool {
	if _, ok := sourceLanguageFiles[path]; ok {
		return true
	}
	for prefix := range sourceLanguageDirectories {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func sourceLanguageRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || r >= 0x3000 && r <= 0x303f ||
		r >= 0xff01 && r <= 0xff0f || r >= 0xff1a && r <= 0xff20 ||
		r >= 0xff3b && r <= 0xff40 || r >= 0xff5b && r <= 0xff65
}

// stripCodeSpans drops Markdown inline code spans. A name quoted verbatim (a Windows account, a
// Mod folder, a path) keeps its own language; the prose around it is still checked.
func stripCodeSpans(line string) string {
	var out strings.Builder
	for i := 0; i < len(line); {
		if line[i] != '`' {
			out.WriteByte(line[i])
			i++
			continue
		}
		n := 0
		for i+n < len(line) && line[i+n] == '`' {
			n++
		}
		end := closingBackticks(line, i+n, n)
		if end < 0 { // An unclosed run is plain text, as in CommonMark.
			out.WriteString(line[i : i+n])
			i += n
			continue
		}
		i = end + n
	}
	return out.String()
}

// closingBackticks finds the next run of exactly n backticks at or after from.
func closingBackticks(line string, from, n int) int {
	for i := from; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := 0
		for i+run < len(line) && line[i+run] == '`' {
			run++
		}
		if run == n {
			return i
		}
		i += run
	}
	return -1
}

func sourceLanguageLines(data []byte, markdown bool) []int {
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return nil
	}
	var lines []int
	for index, line := range strings.Split(string(data), "\n") {
		if markdown {
			line = stripCodeSpans(line)
		}
		for _, r := range line {
			if sourceLanguageRune(r) {
				lines = append(lines, index+1)
				break
			}
		}
	}
	return lines
}

func sourceLanguagePaths(root string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	if data, err := cmd.Output(); err == nil {
		return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00"), nil
	}
	// Source archives and machines without git still get the guard.
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", ".cache", "tmp", "dist", "node_modules", "bin", "obj":
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			paths = append(paths, filepath.ToSlash(relative))
		}
		return err
	})
	return paths, err
}

func TestSourceLanguage(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	paths, err := sourceLanguagePaths(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if path == "" || sourceLanguageExempt(path) {
			continue
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".dll", ".so", ".a", ".o", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".webp", ".woff", ".woff2", ".zip", ".7z", ".pdf":
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if os.IsNotExist(err) {
			continue
		} // Removed tracked files are absent until the changes are committed.
		if err != nil {
			t.Error(err)
			continue
		}
		markdown := strings.EqualFold(filepath.Ext(path), ".md")
		for _, line := range sourceLanguageLines(data, markdown) {
			t.Errorf("%s:%d: CJK text belongs in language resources or documented test data (Markdown may quote a name in `code`)", path, line)
		}
	}
}

func TestSourceLanguageScanner(t *testing.T) {
	for _, r := range []rune{0x4e00, 0x3400, 0x20000, 0x3002, 0xff0c} {
		input := []byte(fmt.Sprintf("English\n%c\n", r))
		for _, markdown := range []bool{false, true} {
			lines := sourceLanguageLines(input, markdown)
			if len(lines) != 1 || lines[0] != 2 {
				t.Fatalf("U+%04X markdown=%v: lines %v", r, markdown, lines)
			}
		}
	}
	if lines := sourceLanguageLines([]byte("English\nASCII !?,.\n"), false); len(lines) != 0 {
		t.Fatal(lines)
	}
	// Code points keep this file free of the text it tests.
	name, other := string([]rune{0x65b0, 0x73a9, 0x5bb6}), string([]rune{0x8001, 0x73a9, 0x5bb6})
	for _, c := range []struct {
		text string
		want []int
	}{
		{"account `" + name + "` signed in", nil},
		{"two `" + name + "` and `" + other + "` spans", nil},
		{"double ``a ` " + name + "`` span", nil},
		{"prose " + name + " outside `" + name + "` a span", []int{1}},
		{"unclosed `" + name + " span", []int{1}},
		{"``" + name + "` wrong run length", []int{1}},
		{"English\n```\n" + name + "\n```\n", []int{3}}, // Fenced blocks stay checked.
	} {
		if got := sourceLanguageLines([]byte(c.text), true); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("markdown %q: lines %v, want %v", c.text, got, c.want)
		}
		if c.want == nil {
			if got := sourceLanguageLines([]byte(c.text), false); len(got) != 1 {
				t.Errorf("code file %q: code spans must not be exempt, got %v", c.text, got)
			}
		}
	}
	if !sourceLanguageExempt("ToDo.md") {
		t.Fatal("the working list may contain Chinese")
	}
	if sourceLanguageExempt("internal/view/future.go") || sourceLanguageExempt("internal/view/new_test.go") {
		t.Fatal("test and code directories must not be exempt")
	}
}
