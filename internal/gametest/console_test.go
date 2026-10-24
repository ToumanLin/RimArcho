package gametest

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

func TestConsoleLog(t *testing.T) {
	var output bytes.Buffer
	c, err := newConsoleLog(&output, "1.7.4985 rev1269", time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	input := "03:36:12  INFO Loading\r\n03:36:12  WARN A warning\r\n" +
		"03:36:12  ERROR An exception\r\n03:36:12    at Example.Mod.Tick()\r\n" +
		"03:36:12 Details\r\nuntimed continuation\r\n" +
		"03:36:12  DEBUG Debug\r\n03:36:12  TRACE Trace\r\n" +
		"03:36:12  FATAL Fatal\r\n03:36:12  CRIT Critical"
	// Split timestamps, messages and CRLF pairs across writes.
	for _, b := range []byte(input) {
		if _, err := c.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(output.String(), "2026-10-24 03:36:12.000  ERROR An exception\n   at Example.Mod.Tick()\nDetails\n") {
		t.Fatalf("streamed output: %q", output.String())
	}
	if err := c.flush(); err != nil {
		t.Fatal(err)
	}
	f, err := logs.Parse(strings.NewReader(output.String()))
	if err != nil {
		t.Fatal(err)
	}
	if !f.Waylith || f.Version != "1.7.4985 rev1269" || len(f.Entries) != 7 {
		t.Fatalf("parsed log: %+v", f)
	}
	for i, level := range []string{"INFO", "WARN", "ERROR", "DEBUG", "TRACE", "FATAL", "CRIT"} {
		if f.Entries[i].Level != level {
			t.Errorf("entry %d: %+v", i, f.Entries[i])
		}
	}
	e := f.Entries[2]
	if e.Message != "An exception" || len(e.Frames) != 1 || e.Frames[0].Type != "Example.Mod" ||
		len(e.Extra) != 2 || e.Extra[0] != "Details" || e.Extra[1] != "untimed continuation" {
		t.Fatalf("exception: %+v", e)
	}
}
