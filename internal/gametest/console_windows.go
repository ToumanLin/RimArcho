package gametest

import (
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// decodeANSI reads a console line the game wrote in the system ANSI code page, which it
// does when the UTF-8 startup hook did not run (20 §2).
func decodeANSI(b []byte) string {
	const acp = 0 // CP_ACP
	if len(b) == 0 {
		return ""
	}
	n, err := windows.MultiByteToWideChar(acp, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n <= 0 {
		return strings.ToValidUTF8(string(b), "�")
	}
	w := make([]uint16, n)
	n, err = windows.MultiByteToWideChar(acp, 0, &b[0], int32(len(b)), &w[0], n)
	if err != nil || n <= 0 {
		return strings.ToValidUTF8(string(b), "�")
	}
	return string(utf16.Decode(w[:n]))
}
