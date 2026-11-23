//go:build !windows

package gametest

import "strings"

// decodeANSI keeps what is readable of a line that is not UTF-8 (20 §2).
func decodeANSI(b []byte) string {
	return strings.ToValidUTF8(string(b), "�")
}
