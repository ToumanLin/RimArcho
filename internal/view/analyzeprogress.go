package view

import (
	"regexp"
	"strconv"
)

// AnalyzeProgress is a line of `archo analyze`'s progress in the interface language (the
// status bar showed "defs: 1000/1559 files" in a Chinese window). Lines it does not know
// stay as they are.
func AnalyzeProgress(line string) string {
	for _, p := range analyzeLines {
		m := p.re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		args := make([]any, 0, len(m)-1)
		for _, s := range m[1:] {
			if n, err := strconv.Atoi(s); err == nil {
				args = append(args, n)
			} else {
				args = append(args, s)
			}
		}
		return Tf(p.format, args...)
	}
	return line
}

var analyzeLines = []struct {
	re     *regexp.Regexp
	format string
}{
	{regexp.MustCompile(`^defs: (\d+)/(\d+) files$`), "defs: %d/%d files"},
	{regexp.MustCompile(`^defs: (\d+) files, (\d+) defs from (\d+) active mods$`), "defs: %d files, %d defs from %d active mods"},
	{regexp.MustCompile(`^defs: (\d+) inactive mods indexed \((\d+) cached\)$`), "defs: %d inactive mods indexed (%d cached)"},
	{regexp.MustCompile(`^patches: (\d+) files, (\d+) operations$`), "patches: %d files, %d operations"},
	{regexp.MustCompile(`^patches: (\d+)/(\d+) operations$`), "patches: %d/%d operations"},
	{regexp.MustCompile(`^done: (\d+) failed operations, (\d+) findings, (\S+)$`), "done: %d failed operations, %d findings, %s"},
}
