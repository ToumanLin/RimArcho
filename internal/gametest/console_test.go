package gametest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

func TestConsoleLog(t *testing.T) {
	var output bytes.Buffer
	c, err := newConsoleLog(&output, "1.7.4985 rev1269", time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	// Levels padded to five characters as Waylith writes them (FATAL: two spaces also parse).
	input := "03:36:12  INFO Loading\r\n03:36:12  WARN A warning\r\n" +
		"03:36:12 ERROR An exception\r\n03:36:12    at Example.Mod.Tick()\r\n" +
		"03:36:12 Details\r\nuntimed continuation\r\n" +
		"03:36:12 DEBUG Debug\r\n03:36:12 TRACE Trace\r\n" +
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

// TestConsoleLogEncoding covers the decoding path of 20 \u00a72: UTF-8 from the startup hook stays
// as it is, and a line in the ANSI code page (the hook did not run) is decoded, not mangled.
// The input is Chinese, Japanese and Thai Mod text, plus a line in Windows-1252.
func TestConsoleLogEncoding(t *testing.T) {
	var output bytes.Buffer
	c, err := newConsoleLog(&output, "1.7.5014 rev1195", time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	c.decode = func(b []byte) string {
		s, err := charmap.Windows1252.NewDecoder().Bytes(b)
		if err != nil {
			t.Fatal(err)
		}
		return string(s)
	}
	input := "10:00:00  INFO Loading \u4e2d\u6587\u6a21\u7ec4\u540d \u65e5\u672c\u8a9e\u306eMod \u0e44\u0e17\u0e22\r\n" +
		"10:00:01 ERROR Exception in \u6c49\u5316\u8865\u4e01.Patch: \u9519\u8bef\r\n" +
		"10:00:01    at \u4e2d\u6587.Mod.Tick()\r\n" +
		"10:00:02  WARN Caf\xe9 Mod \xab1252\xbb\r\n"
	// Split multi-byte characters across writes.
	for _, b := range []byte(input) {
		if _, err := c.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.flush(); err != nil {
		t.Fatal(err)
	}
	if !utf8.Valid(output.Bytes()) {
		t.Fatalf("output is not UTF-8: %q", output.String())
	}
	f, err := logs.Parse(strings.NewReader(output.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Entries) != 3 {
		t.Fatalf("parsed log: %+v", f)
	}
	for i, want := range []string{"Loading \u4e2d\u6587\u6a21\u7ec4\u540d \u65e5\u672c\u8a9e\u306eMod \u0e44\u0e17\u0e22", "Exception in \u6c49\u5316\u8865\u4e01.Patch: \u9519\u8bef", "Caf\u00e9 Mod \u00ab1252\u00bb"} {
		if f.Entries[i].Message != want {
			t.Errorf("entry %d: %q, want %q", i, f.Entries[i].Message, want)
		}
	}
	if fr := f.Entries[1].Frames; len(fr) != 1 || fr[0].Type != "\u4e2d\u6587.Mod" {
		t.Errorf("frames: %+v", fr)
	}
}

// TestConsoleHookBuilt fails when StartupHook.cs changed without consolehook/build.ps1.
func TestConsoleHookBuilt(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("consolehook", "StartupHook.cs"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("consolehook", "source.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n")))
	if got := hex.EncodeToString(sum[:]); got != strings.TrimSpace(string(want)) {
		t.Fatalf("StartupHook.cs hashes to %s, built from %s: run pwsh internal/gametest/consolehook/build.ps1", got, want)
	}
	if !bytes.Contains(consoleHook, []byte("StartupHook")) || !bytes.Contains(consoleHook, []byte("set_OutputEncoding")) {
		t.Fatal("embedded hook lacks StartupHook.Initialize setting Console.OutputEncoding")
	}
}
