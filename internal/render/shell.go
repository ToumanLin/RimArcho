package render

import (
	"strings"

	"github.com/ToumanLin/RimArcho/internal/ids"
)

// ShellSafe writes the instance references of the archo commands in s without "#"
// ("fix: archo enable #nyh3" → "fix: archo enable nyh3"), 13 §3.1 rule 11. A word that
// starts with "#" is a comment in PowerShell and bash, so a pasted command would run
// without its argument. The bare ID prefix is accepted wherever a reference is (13 §2)
// and needs no quotes in cmd, PowerShell or bash. A "#" inside a word (L#7c2e,
// decl:#iqmu:…), inside double quotes, and every reference outside a command keep theirs.
//
// A command starts at the word "archo" and runs to the end of the line, or stops at the
// ")" that closes the bracket it is in, at ";", or at two spaces (the column gap).
func ShellSafe(s string) string {
	if !strings.Contains(s, "archo ") || !strings.Contains(s, "#") {
		return s
	}
	var sb strings.Builder
	last := 0
	for _, sp := range CommandSpans(s) {
		sb.WriteString(s[last:sp[0]])
		sb.WriteString(bareRefs(s[sp[0]:sp[1]]))
		last = sp[1]
	}
	sb.WriteString(s[last:])
	return sb.String()
}

// CommandSpans returns the [start, end) byte ranges of the archo commands in s.
func CommandSpans(s string) [][2]int {
	var out [][2]int
	for off := 0; ; {
		i := strings.Index(s[off:], "archo ")
		if i < 0 {
			return out
		}
		i += off
		if i > 0 && strings.IndexByte(" \t\n([`'\"", s[i-1]) < 0 {
			off = i + 1 // "rimarcho ", not a command
			continue
		}
		end := commandEnd(s, i)
		out = append(out, [2]int{i, end})
		off = end
	}
}

// commandEnd is where the command that starts at i ends.
func commandEnd(s string, i int) int {
	depth, quoted := 0, false
	for j := i + len("archo "); j < len(s); j++ {
		c := s[j]
		switch {
		case quoted:
			quoted = c != '"'
		case c == '"':
			quoted = true
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return j
			}
			depth--
		case c == ';' || c == '\n' || c == ' ' && j+1 < len(s) && s[j+1] == ' ':
			return j
		}
	}
	return len(s)
}

// bareRefs drops the "#" of every unquoted word "#<id>" in one command.
func bareRefs(cmd string) string {
	var sb strings.Builder
	quoted := false
	for j := 0; j < len(cmd); j++ {
		c := cmd[j]
		if c == '"' {
			quoted = !quoted
		}
		if c == '#' && !quoted && (j == 0 || cmd[j-1] == ' ') {
			k := strings.IndexByte(cmd[j:], ' ')
			if k < 0 {
				k = len(cmd) - j
			}
			if ids.Valid(cmd[j+1 : j+k]) {
				continue
			}
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
