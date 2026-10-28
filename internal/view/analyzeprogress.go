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

func init() {
	Register(map[string]string{
		"defs: %d/%d files":                           "Def 文件 %d/%d",
		"defs: %d files, %d defs from %d active mods": "%[3]d 个启用的 Mod：%[1]d 个文件，%[2]d 个 Def",
		"defs: %d inactive mods indexed (%d cached)":  "已索引 %d 个未启用的 Mod（%d 个来自缓存）",
		"patches: %d files, %d operations":            "Patch：%d 个文件，%d 个操作",
		"patches: %d/%d operations":                   "Patch 操作 %d/%d",
		"done: %d failed operations, %d findings, %s": "完成：%d 个操作失败，%d 个发现，用时 %s",
	})
}
