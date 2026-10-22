package app

import (
	"regexp"
	"strings"
	"testing"
)

// packNameOK is the rule the pack name regexp used to be.
func TestPackNameSameAsRegexp(t *testing.T) {
	re := regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} ._\-]{0,63}$`)
	cases := []string{"", "a", "整合包", "1.5 raid", ".x", " x", "-x", "x-", "x/y", "a\x80b", "Ⅻ", "x²",
		strings.Repeat("a", 64), strings.Repeat("a", 65), strings.Repeat("包", 64), strings.Repeat("包", 65),
		"a\tb", "a_b.c-d e", "🙂", "x" + strings.Repeat(" ", 63)}
	for _, c := range cases {
		if re.MatchString(c) != packNameOK(c) {
			t.Errorf("%q: regexp %v, packNameOK %v", c, re.MatchString(c), packNameOK(c))
		}
	}
}
