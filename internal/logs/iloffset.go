package logs

import (
	"regexp"
	"strconv"
)

// "at Ns.Type.Method (args) [0x00076] in <hash>:0": the IL offset of a Mono frame.
var ilOffsetRe = regexp.MustCompile(`\) \[0x([0-9A-Fa-f]{1,8})\](?: in <[^>]*>:\d+)?\s*$`)

// ilOffset reads a frame's IL offset.
func ilOffset(text string) (int, bool) {
	m := ilOffsetRe.FindStringSubmatch(text)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseInt(m[1], 16, 64)
	if err != nil {
		return 0, false
	}
	return int(n), true
}
