package source

import (
	"errors"
	"testing"
)

// ErrTestRecycle: a test binary asked for the real recycle bin (17 §4). Tests recycle into
// a fake bin instead (testkit gives every App one); a test reaching the real bin would move
// its synthetic folders into the player's own Recycle Bin.
var ErrTestRecycle = errors.New("a test must not use the real recycle bin: give the App a TrashFunc (testkit.Env does)")

// realBinRefused is true in a test binary, where Trash refuses before reaching the shell.
func realBinRefused() bool { return testing.Testing() }
