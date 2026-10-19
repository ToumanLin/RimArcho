package analyze

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LogFailure is one "Patch operation ... failed" entry of Player.log.
type LogFailure struct {
	Mod, Desc, File string
}

// Key compares a failure across the log and the simulation: file path + description.
func (f LogFailure) Key() string { return FailKey(f.File, f.Desc) }

var logHead = regexp.MustCompile(`^\[(.*?)\] Patch operation (.*)$`)

// ParseLogFailures extracts the patch failures from a Player.log (the M0 spike's parser:
// descriptions can span lines, and the next line names the file).
func ParseLogFailures(path string) ([]LogFailure, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	var out []LogFailure
	var cur *LogFailure
	var desc strings.Builder
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if cur != nil {
			if desc.Len() > 0 {
				desc.WriteString("\n" + line)
				if strings.HasSuffix(line, " failed") {
					cur.Desc = strings.TrimSuffix(desc.String(), " failed")
					desc.Reset()
				}
				continue
			}
			if strings.HasPrefix(line, "file: ") {
				cur.File = strings.TrimPrefix(line, "file: ")
			}
			out = append(out, *cur)
			cur = nil
			if strings.HasPrefix(line, "file: ") {
				continue
			}
		}
		if m := logHead.FindStringSubmatch(line); m != nil {
			cur = &LogFailure{Mod: m[1]}
			if strings.HasSuffix(m[2], " failed") {
				cur.Desc = strings.TrimSuffix(m[2], " failed")
			} else {
				desc.WriteString(m[2])
			}
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out, sc.Err()
}

var wsRe = regexp.MustCompile(`\s+`)

// FailKey normalises a file path and an operation description.
func FailKey(file, desc string) string {
	return NormPath(file) + "|" + wsRe.ReplaceAllString(strings.TrimSpace(desc), " ")
}

// NormPath lower-cases and cleans a Windows path.
func NormPath(p string) string {
	return strings.ToLower(filepath.Clean(strings.ReplaceAll(p, "/", `\`)))
}
