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

func TestDescriptionStamp(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"[h3]RimArcho[/h3]\r\nText", "[h3]RimArcho[/h3]\r\nText\n\nRimArcho v1.2.3"},
		{"Text\n\nRimArcho v1.2.2\n", "Text\n\nRimArcho v1.2.3"},                // the file already carries an older stamp
		{"Text\nRimArcho v1.2.2\r\nRimArcho v1.2.1", "Text\n\nRimArcho v1.2.3"}, // repeated stamps collapse
		{"RimArcho v1.2.2", "RimArcho v1.2.3"},
		{"", "RimArcho v1.2.3"},
	} {
		got := Stamp(tc.in, "1.2.3")
		if got != tc.want {
			t.Errorf("Stamp(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if v := DescriptionVersion(got); v != "1.2.3" {
			t.Errorf("DescriptionVersion(%q) = %q", got, v)
		}
	}
	for _, tc := range []struct{ in, want string }{
		{"RimArcho v1.0.5 added X.\nRimArcho v1.0.6\nedited on the web", "1.0.6"}, // the last stamp line counts
		{"Text\r\n\r\n  RimArcho v1.2.10  \r\n", "1.2.10"},
		{"RimArcho v1.0.5 added the Workshop item", ""},
		{"RimArcho v1.2.3-rc1\nRimArcho v1.2\nRimArcho v01.2.3\nRimArcho v1.2.3+build\nRimArcho 1.2.3", ""},
		{"", ""},
	} {
		if got := DescriptionVersion(tc.in); got != tc.want {
			t.Errorf("DescriptionVersion(%q) = %q, want %q", tc.in, got, tc.want)
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

func TestSubscribedVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"archo-0.99.1-windows-amd64.zip", "archo-0.99.1-windows-amd64.zip.sha256",
		"archo-0.99.3-windows-amd64.zip", // no checksum: ignored
		"archo-1.0.0-rc1-windows-amd64.zip", "archo-1.0.0-rc1-windows-amd64.zip.sha256",
		"archo-0.99.2-linux-amd64.zip", "archo-0.99.2-linux-amd64.zip.sha256",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := SubscribedVersion(dir, "windows", "amd64"); got != "0.99.1" {
		t.Fatalf("got %q", got)
	}
	if got := SubscribedVersion(filepath.Join(dir, "missing"), "windows", "amd64"); got != "" {
		t.Fatalf("missing folder: %q", got)
	}
}
