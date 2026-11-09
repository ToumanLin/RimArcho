package app

import (
	"strings"
	"testing"
)

// ilspycmd 9-11 print an update notice on stdout, and "Could not find a type named" with exit
// code 0 for a type that is not in the assembly; neither is source.
func TestCleanILSpy(t *testing.T) {
	src := "namespace Verse;\n\npublic class TickList\n{\n}\nYou are not using the latest version of the tool, please update.\r\nLatest version is '11.2.0.10050' (yours is '9.1.0.7988')\r\n"
	out, ok := cleanILSpy([]byte(src))
	if !ok || strings.Contains(string(out), "latest version") || !strings.HasSuffix(string(out), "}\n") {
		t.Fatalf("cleaned %q %v", out, ok)
	}
	for _, s := range []string{
		"You are not using the latest version of the tool, please update.\nLatest version is '11.2.0.10050' (yours is '9.1.0.7988')\n",
		"Could not find a type named 'RimWorld.PlayLogEntry_Interaction'.\n",
		"",
	} {
		if out, ok := cleanILSpy([]byte(s)); ok || len(out) != 0 {
			t.Errorf("%q: kept %q, type %v", s, out, ok)
		}
	}
}
