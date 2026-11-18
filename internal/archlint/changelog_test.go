package archlint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var changelogVersion = regexp.MustCompile(`^## (\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?(?:$| )`)

// changelogProblems: the first "## " heading is "## Unreleased", where every change goes
// (AGENTS.md); the rest are released versions, unique and newest first (17 §5).
func changelogProblems(text string) []string {
	var out []string
	var prev []int
	prevName := ""
	seen := map[string]bool{}
	first := true
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		at := fmt.Sprintf("CHANGELOG.md:%d", i+1)
		if first {
			first = false
			if strings.TrimSpace(line) != "## Unreleased" {
				out = append(out, fmt.Sprintf("AGENTS.md, 17 §5: %s: the first version heading is %q; it must be \"## Unreleased\", where new entries go until a release renames it", at, line))
			}
			continue
		}
		m := changelogVersion.FindStringSubmatch(line)
		if m == nil {
			out = append(out, fmt.Sprintf("17 §5: %s: %q is not a version heading (\"## X.Y.Z\" or \"## X.Y.Z-rc1\", then optional text); only the first heading is Unreleased", at, line))
			continue
		}
		name := m[1] + "." + m[2] + "." + m[3] + m[4]
		if seen[name] {
			out = append(out, fmt.Sprintf("17 §5: %s: version %s appears twice", at, name))
		}
		seen[name] = true
		v := make([]int, 3)
		for j := range v {
			v[j], _ = strconv.Atoi(m[j+1])
		}
		if prev != nil && !newer(prev, prevName, v, m[4]) {
			out = append(out, fmt.Sprintf("17 §5: %s: %s comes after %s; versions go newest first", at, name, prevName))
		}
		prev, prevName = v, name
	}
	if first {
		out = append(out, "AGENTS.md: CHANGELOG.md has no \"## Unreleased\" heading")
	}
	return out
}

// newer reports whether version a (with prerelease suffix pa, from prevName) is newer than b.
func newer(a []int, prevName string, b []int, pb string) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	pa := ""
	if i := strings.IndexByte(prevName, '-'); i >= 0 {
		pa = prevName[i:]
	}
	switch {
	case pa == pb:
		return false
	case pa == "": // 1.1.0 is newer than 1.1.0-rc1
		return true
	case pb == "":
		return false
	}
	return pa > pb
}

func TestArchChangelog(t *testing.T) {
	t.Parallel()
	root, _ := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	report(t, changelogProblems(string(b)))
}

func TestArchChangelogDetection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want int
	}{
		{"# Changelog\n\n## Unreleased\n### Added\n## 1.1.0 — 2026-12-01\n## 1.1.0-rc2\n## 1.1.0-rc1\n## 1.0.9 — x\n## 0.9.0 — plans (milestone)\n", 0},
		{"# Changelog\n## 1.1.0\n## 1.0.0\n", 1},
		{"## Unreleased\n## 1.0.0\n## 1.0.1\n", 1},
		{"## Unreleased\n## 1.0.0\n## 1.0.0 — again\n", 2},
		{"## Unreleased\n## Next\n## 1.0.0-rc1\n## 1.0.0\n", 2},
		{"# nothing\n", 1},
	} {
		expect(t, strconv.Quote(tc.text), changelogProblems(tc.text), tc.want)
	}
}
