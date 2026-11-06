package selfupdate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkshopVersion(t *testing.T) {
	for _, tc := range []struct {
		tags []string
		want string
	}{
		{[]string{"1.6", "Mod", "v1.2.3"}, "1.2.3"},
		{[]string{"v1.2.2", "v1.2.10", "v1.2.3"}, "1.2.10"},
		{[]string{"v1.2", "v1.02.3", "1.2.3", "v2.0.0-rc1"}, ""},
		{nil, ""},
	} {
		if got := WorkshopVersion(tc.tags); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.tags, got, tc.want)
		}
	}
}

func TestCopyLocal(t *testing.T) {
	r := &Release{Zip: Asset{Name: ZipName("1.2.3", "windows", "amd64"), Size: 999}, WorkshopItem: "8100"}
	r.Sum.Name = r.Zip.Name + ".sha256"
	source, dir := t.TempDir(), t.TempDir()
	data := []byte("synthetic release zip")
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(source, name), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(r.Zip.Name, data)
	write(r.Sum.Name, []byte(sumOf(data)+"  "+r.Zip.Name+"\n"))
	path, err := CopyLocal(context.Background(), r, source, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != string(data) {
		t.Fatal(string(b), err)
	}
	// A local file's checksum is authoritative, but GitHub's declared size is checked.
	if err := VerifyLocal(context.Background(), path, r.Zip.Name, []byte(sumOf(data)), 999); err == nil {
		t.Fatal("ignored GitHub size")
	}
	write(r.Zip.Name, []byte("truncated"))
	if _, err := CopyLocal(context.Background(), r, source, dir); !errors.Is(err, ErrChecksum) {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("retained corrupt download", err)
	}
	if _, err := os.Stat(filepath.Join(source, r.Zip.Name)); err != nil {
		t.Fatal("changed subscribed files", err)
	}
}
