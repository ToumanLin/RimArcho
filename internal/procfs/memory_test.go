package procfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatus(t *testing.T) {
	m, err := Status(strings.NewReader("Name:\tfixture\nVmSize: 9000 kB\nVmRSS:\t2048 kB\nVmHWM: 4096 kB\nRssAnon: 1024 kB\n"))
	if err != nil || m != (Memory{RSS: 2 << 20, Private: 1 << 20, Peak: 4 << 20}) {
		t.Fatalf("%+v: %v", m, err)
	}
	m, err = Status(strings.NewReader("Name: kernel-thread\n"))
	if err != nil || m != (Memory{}) {
		t.Fatalf("missing counters: %+v, %v", m, err)
	}
	for _, text := range []string{"VmRSS: -1 kB", "VmRSS: 2 MB", "VmRSS: nope kB", "VmRSS: 9007199254740992 kB", "VmRSS:"} {
		if _, err := Status(strings.NewReader(text)); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	if _, err := Status(brokenReader{}); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("reader error lost: %v", err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestStatusAt(t *testing.T) {
	root := t.TempDir()
	for _, pid := range []string{"self", "1234"} {
		dir := filepath.Join(root, pid)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte("VmRSS: 2048 kB\nVmHWM: 3072 kB\nRssAnon: 1024 kB\n"), 0600); err != nil {
			t.Fatal(err)
		}
		m, err := StatusAt(root, pid)
		if err != nil || m.RSS != 2<<20 || m.Peak != 3<<20 || m.Private != 1<<20 {
			t.Fatalf("%s: %+v %v", pid, m, err)
		}
	}
	if m, err := StatusAt(root, "exited"); err == nil || m != (Memory{}) {
		t.Fatalf("exited: %+v %v", m, err)
	}
}
