package app_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/klauspost/compress/zstd"
)

type archiveTestEntry struct {
	name, body string
	kind       byte
}

func writeTestPackArchive(t *testing.T, file string, p app.Pack, zipFormat bool, entries []archiveTestEntry) {
	t.Helper()
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	entries = append([]archiveTestEntry{{name: "pack.json", body: string(b)}}, entries...)
	if zipFormat {
		zw := zip.NewWriter(f)
		for _, e := range entries {
			h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
			switch e.kind {
			case tar.TypeSymlink:
				h.SetMode(os.ModeSymlink | 0o777)
			case tar.TypeDir:
				h.SetMode(os.ModeDir | 0o755)
			default:
				h.SetMode(0o644)
			}
			w, err := zw.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, e.body); err != nil {
				t.Fatal(err)
			}
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		zw, err := zstd.NewWriter(f, zstd.WithEncoderConcurrency(1))
		if err != nil {
			t.Fatal(err)
		}
		tw := tar.NewWriter(zw)
		for _, e := range entries {
			kind := e.kind
			if kind == 0 {
				kind = tar.TypeReg
			}
			h := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.body)), Typeflag: kind}
			if kind == tar.TypeSymlink {
				h.Linkname = "outside"
			}
			if err := tw.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(tw, e.body); err != nil {
				t.Fatal(err)
			}
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func testCopyHash(t *testing.T, dir string) (int, string) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(dir, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".rimarcho" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, rel := range paths {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(h, "%s\x00%d\x00%x\n", rel, len(b), sum)
	}
	return len(paths), hex.EncodeToString(h.Sum(nil))
}

func assertNoPackTemp(t *testing.T, packsDir, name string) {
	t.Helper()
	for _, file := range []string{name, name + app.TmpSuffix} {
		if _, err := os.Lstat(filepath.Join(packsDir, file)); !os.IsNotExist(err) {
			t.Fatalf("failed import left %s: %v", file, err)
		}
	}
}

func TestPackArchiveRoundTrip(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("enable", "pkg:tester.versions")
	e.MustRun("pack", "build", "roundtrip")
	p, err := e.App.ReadPack("roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	if p.Format != 2 {
		t.Fatalf("format: %d", p.Format)
	}
	dir := filepath.Join(e.App.PacksDir(), p.Name)
	var carried app.PackEntry
	for _, entry := range p.Entries {
		if entry.Official {
			continue
		}
		carried = entry
		n, sum := testCopyHash(t, filepath.Join(dir, "mods", entry.Folder))
		if entry.Files != n || entry.SHA256 != sum {
			t.Fatalf("build digest: %+v; want %d %s", entry, n, sum)
		}
	}
	mod := filepath.Join(dir, "mods", carried.Folder)
	// Copy provenance is carried, but deliberately excluded from the digest.
	e.WriteFile(filepath.Join(mod, ".rimarcho", "note"), "provenance")
	modified := time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(mod, "About", "About.xml"), modified, modified); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"userdata/Config/secret.xml", "state.json", "build.json", "other.txt"} {
		e.WriteFile(filepath.Join(dir, filepath.FromSlash(file)), "private")
	}
	file := filepath.Join(e.Root, "roundtrip.archopack")
	e.MustRun("pack", "export", p.Name, file)
	if !app.IsPackZip(file) {
		t.Fatal("GUI pack detection did not recognize zstd")
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte{0x28, 0xb5, 0x2f, 0xfd}) {
		t.Fatal("archive does not start with zstd magic")
	}
	zr, err := zstd.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	h, err := tr.Next()
	if err != nil || h.Name != "pack.json" {
		t.Fatalf("first tar entry: %v %v", h, err)
	}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(h.Name, "mods/") {
			t.Fatalf("private pack file was exported: %s", h.Name)
		}
		if h.Name == "mods/"+carried.Folder+"/About/About.xml" && !h.ModTime.Equal(modified) {
			t.Fatalf("mtime lost: %v", h.ModTime)
		}
	}
	zr.Close()
	checked, problems, err := e.App.VerifyPackFile(file)
	if err != nil || len(problems) != 0 || checked.Format != 2 {
		t.Fatalf("stream verify: %v %+v", err, problems)
	}
	if out := e.MustRun("pack", "check", file); !strings.Contains(out, "verified") {
		t.Fatal(out)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ImportPack(file, false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Verified || !reflect.DeepEqual(p.Entries, r.Pack.Entries) {
		t.Fatalf("roundtrip manifest: %+v", r)
	}
	for _, entry := range r.Pack.Entries {
		if entry.Official {
			continue
		}
		n, sum := testCopyHash(t, filepath.Join(r.Dir, "mods", entry.Folder))
		if n != entry.Files || sum != entry.SHA256 {
			t.Fatalf("roundtrip digest: %s", entry.Name)
		}
	}
	for _, file := range []string{"userdata", "state.json", "build.json", "other.txt"} {
		if _, err := os.Stat(filepath.Join(r.Dir, file)); !os.IsNotExist(err) {
			t.Fatalf("private data imported: %s", file)
		}
	}
	if _, err := os.Stat(file + ".part"); !os.IsNotExist(err) {
		t.Fatal("export left .part")
	}
}

func TestPackImportFormat1(t *testing.T) {
	e := packEnv(t)
	p := app.Pack{Format: 1, Name: "legacy", Entries: []app.PackEntry{{PackageID: "test.legacy", Name: "Legacy Mod", Folder: "legacy"}}}
	file := filepath.Join(e.Root, "legacy.archopack")
	writeTestPackArchive(t, file, p, true, []archiveTestEntry{{name: "mods/legacy/About/About.xml", body: "old bytes"}})
	r, err := e.App.ImportPack(file, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.Verified {
		t.Fatal("old-format import claimed verification")
	}
	if out := e.MustRun("pack", "check", p.Name); !strings.Contains(out, "not verified (old format)") {
		t.Fatal(out)
	}
	// Export upgrades an installed old pack, atomically recording its new digests.
	upgraded := filepath.Join(e.Root, "upgraded.archopack")
	if _, err := e.App.ExportPack(p.Name, upgraded, false); err != nil {
		t.Fatal(err)
	}
	got, err := e.App.ReadPack(p.Name)
	if err != nil || got.Format != 2 || got.Entries[0].Files != 1 || len(got.Entries[0].SHA256) != 64 {
		t.Fatalf("upgrade: %+v %v", got, err)
	}
	if _, problems, err := e.App.VerifyPackFile(upgraded); err != nil || len(problems) > 0 {
		t.Fatalf("upgraded verify: %+v %v", problems, err)
	}
	got.Entries[0].WorkshopUpdated, got.Entries[0].GitHead = 123456, "abcdef"
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	e.WriteFile(filepath.Join(r.Dir, "pack.json"), string(b))
	list := filepath.Join(e.Root, "legacy.archolist.json")
	if _, err := e.App.ExportPack(p.Name, list, true); err != nil {
		t.Fatal(err)
	}
	b, err = os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	var light app.Pack
	if err := json.Unmarshal(b, &light); err != nil {
		t.Fatal(err)
	}
	le := light.Entries[0]
	if !light.Light || le.Folder != "" || le.Files != 0 || le.SHA256 != "" || le.Bytes != 0 || le.WorkshopUpdated != 123456 || le.GitHead != "abcdef" {
		t.Fatalf("list metadata: %+v", le)
	}
}

func TestPackImportRejectsGameFiles(t *testing.T) {
	e := packEnv(t)
	for _, zipFormat := range []bool{false, true} {
		for _, tc := range []struct {
			name  string
			entry archiveTestEntry
			mod   app.PackEntry
		}{
			{name: "game-data", entry: archiveTestEntry{name: "RimWorldWin64_Data/x", body: "game"}},
			{name: "core", entry: archiveTestEntry{name: "Data/Core/About/About.xml", body: "game"}},
			{name: "version", entry: archiveTestEntry{name: "Version.txt", body: "1.6"}},
			{name: "symlink", entry: archiveTestEntry{name: "mods/demo/link", kind: tar.TypeSymlink}},
			{name: "hardlink", entry: archiveTestEntry{name: "mods/demo/link", kind: tar.TypeLink}},
			{name: "carried-game", mod: app.PackEntry{PackageID: "LuDeOn.RimWorld.royalty", Name: "Royalty", Folder: "demo"}},
			{name: "official-folder", mod: app.PackEntry{PackageID: "ludeon.rimworld", Name: "Core", Folder: "demo", Official: true}},
			{name: "extra-mod", entry: archiveTestEntry{name: "mods/extra/", kind: tar.TypeDir}},
			{name: "traversal", entry: archiveTestEntry{name: "mods/demo/../../escape", body: "escape"}},
			{name: "absolute", entry: archiveTestEntry{name: "/mods/demo/escape", body: "escape"}},
			{name: "drive", entry: archiveTestEntry{name: "C:/escape", body: "escape"}},
			{name: "backslash", entry: archiveTestEntry{name: "mods/demo/..\\escape", body: "escape"}},
			{name: "duplicate", entry: archiveTestEntry{name: "mods/demo/file", body: "duplicate"}},
		} {
			t.Run(fmt.Sprintf("%s/zip=%v", tc.name, zipFormat), func(t *testing.T) {
				if zipFormat && tc.name == "hardlink" {
					t.Skip("ZIP has no hardlink entry type")
				}
				format := 2
				if zipFormat {
					format = 1
				}
				mod := app.PackEntry{PackageID: "test.demo", Name: "Demo", Folder: "demo"}
				if tc.mod.PackageID != "" {
					mod = tc.mod
				}
				p := app.Pack{Format: format, Name: "rejected", Entries: []app.PackEntry{mod}}
				entries := []archiveTestEntry{{name: "mods/demo/file", body: "demo"}}
				if tc.entry.name != "" {
					entries = append(entries, tc.entry)
				}
				file := filepath.Join(e.Root, "rejected.archopack")
				writeTestPackArchive(t, file, p, zipFormat, entries)
				if _, err := e.App.ImportPack(file, false); err == nil {
					t.Fatal("unsafe archive imported")
				}
				if _, _, err := e.App.VerifyPackFile(file); err == nil {
					t.Fatal("unsafe archive passed streaming whitelist")
				}
				assertNoPackTemp(t, e.App.PacksDir(), p.Name)
			})
		}
	}
}

func TestPackImportVerifies(t *testing.T) {
	e := packEnv(t)
	e.MustRun("disable", "pkg:gone.mod")
	e.MustRun("pack", "build", "verified")
	p, err := e.App.ReadPack("verified")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(e.Root, "verified.archopack")
	if _, err := e.App.ExportPack(p.Name, file, false); err != nil {
		t.Fatal(err)
	}
	zf, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zstd.NewReader(zf)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var entries []archiveTestEntry
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if h.Name != "pack.json" {
			entries = append(entries, archiveTestEntry{h.Name, string(b), h.Typeflag})
		}
	}
	zr.Close()
	zf.Close()
	var target app.PackEntry
	for _, mod := range p.Entries {
		if !mod.Official {
			target = mod
			break
		}
	}
	targetPath := "mods/" + target.Folder + "/About/About.xml"
	var changed []archiveTestEntry
	for _, entry := range entries {
		if entry.name == targetPath {
			entry.body += "tampered"
		}
		changed = append(changed, entry)
	}
	bad := filepath.Join(e.Root, "tampered.archopack")
	q := *p
	q.Name = "tampered"
	writeTestPackArchive(t, bad, q, false, changed)
	if _, err := e.App.ImportPack(bad, false); err == nil || !strings.Contains(err.Error(), target.Name) {
		t.Fatalf("tampered import: %v", err)
	}
	assertNoPackTemp(t, e.App.PacksDir(), q.Name)
	if _, problems, err := e.App.VerifyPackFile(bad); err != nil || len(problems) != 1 || problems[0].PackageID != target.PackageID {
		t.Fatalf("tampered streaming verify: %+v %v", problems, err)
	}
	if out, code := e.Run("pack", "check", bad); code == 0 || !strings.Contains(out, "blocked") || !strings.Contains(out, target.Name) || !strings.Contains(out, "content differs") {
		t.Fatalf("exit %d: %s", code, out)
	}
	// Missing carried folders are also named, even in the old format.
	for _, format := range []int{1, 2} {
		missing := q
		missing.Format = format
		missing.Name = "missing"
		writeTestPackArchive(t, bad, missing, format == 1, nil)
		if _, err := e.App.ImportPack(bad, false); err == nil || !strings.Contains(err.Error(), target.Name) {
			t.Fatalf("missing import: %v", err)
		}
		assertNoPackTemp(t, e.App.PacksDir(), missing.Name)
	}
	installedFile := filepath.Join(e.App.PacksDir(), p.Name, filepath.FromSlash(targetPath))
	e.WriteFile(installedFile, "changed installed bytes")
	problems, err := e.App.VerifyPack(p.Name)
	if err != nil || len(problems) != 1 || problems[0].Name != target.Name {
		t.Fatalf("installed verify: %+v %v", problems, err)
	}
	if out, code := e.Run("pack", "check", p.Name); code == 0 || !strings.Contains(out, "blocked") || !strings.Contains(out, target.Name) {
		t.Fatalf("exit %d: %s", code, out)
	}
	refused := filepath.Join(e.Root, "refused.archopack")
	if _, err := e.App.ExportPack(p.Name, refused, false); err == nil || !strings.Contains(err.Error(), target.Name) {
		t.Fatalf("tampered export: %v", err)
	}
	if _, err := os.Stat(refused); !os.IsNotExist(err) {
		t.Fatal("failed export left output")
	}
	if _, err := os.Stat(refused + ".part"); !os.IsNotExist(err) {
		t.Fatal("failed export left .part")
	}
}

func TestPackArchivePreflight(t *testing.T) {
	e := packEnv(t)
	p := app.Pack{Format: 1, Name: "preflight", Entries: []app.PackEntry{{PackageID: "test.preflight", Name: "Preflight Mod", Folder: "demo", Bytes: 123}}}
	file := filepath.Join(e.Root, "preflight.archopack")
	entries := []archiveTestEntry{{name: "mods/demo/file", body: "bytes"}}
	writeTestPackArchive(t, file, p, true, entries)
	e.App.FreeSpace = func(string) int64 { return app.SpaceMargin + 122 }
	if _, err := e.App.ImportPack(file, false); !errors.Is(err, app.ErrNoSpace) {
		t.Fatalf("space preflight: %v", err)
	}
	assertNoPackTemp(t, e.App.PacksDir(), p.Name)
	e.App.FreeSpace = func(string) int64 { return app.SpaceMargin + 123 }
	if _, err := e.App.ImportPack(file, false); err != nil {
		t.Fatalf("exact space boundary: %v", err)
	}
	if _, err := e.App.ImportPack(file, false); err == nil {
		t.Fatal("import overwrote existing pack")
	}
	if err := os.RemoveAll(filepath.Join(e.App.PacksDir(), p.Name)); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(e.App.PacksDir(), p.Name+app.TmpSuffix)
	writeBuild(t, tmp, app.BuildProgress{Name: p.Name, PID: os.Getpid(), Beat: time.Now()})
	if _, err := e.App.ImportPack(file, false); !errors.Is(err, app.ErrBuildRunning) {
		t.Fatalf("running build preflight: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, app.BuildFile)); err != nil {
		t.Fatal("import changed running build")
	}
	if err := os.RemoveAll(tmp); err != nil {
		t.Fatal(err)
	}
	for _, format := range []int{0, 3} {
		q := p
		q.Format = format
		writeTestPackArchive(t, file, q, true, entries)
		if _, err := e.App.ImportPack(file, false); err == nil || !strings.Contains(err.Error(), "format") {
			t.Fatalf("format %d: %v", format, err)
		}
		assertNoPackTemp(t, e.App.PacksDir(), p.Name)
	}
	q := p
	q.Name = "../escape"
	writeTestPackArchive(t, file, q, false, entries)
	if _, err := e.App.ImportPack(file, false); err == nil {
		t.Fatal("unsafe pack name accepted")
	}
	q = p
	q.Entries = append([]app.PackEntry(nil), p.Entries...)
	q.Entries[0].Bytes = -1
	writeTestPackArchive(t, file, q, false, entries)
	if _, err := e.App.ImportPack(file, false); err == nil {
		t.Fatal("negative copy size accepted")
	}
	assertNoPackTemp(t, e.App.PacksDir(), p.Name)
}

func TestPackArchiveHeaderLimits(t *testing.T) {
	e := packEnv(t)
	p := app.Pack{Format: 2, Name: "limits", Entries: []app.PackEntry{{PackageID: "test.limits", Name: "Limits Mod", Folder: "demo"}}}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		header   tar.Header
		manifest bool
		want     string
	}{
		{"manifest-size", tar.Header{Name: "pack.json", Size: (64 << 20) + 1}, false, "64 MB"},
		{"file-size", tar.Header{Name: "mods/demo/huge", Size: (4 << 30) + 1}, true, "4 GB"},
		{"manifest-order", tar.Header{Name: "mods/demo/"}, false, "first tar entry"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(e.Root, tc.name+".archopack")
			f, err := os.Create(file)
			if err != nil {
				t.Fatal(err)
			}
			zw, err := zstd.NewWriter(f, zstd.WithEncoderConcurrency(1))
			if err != nil {
				t.Fatal(err)
			}
			tw := tar.NewWriter(zw)
			if tc.manifest {
				if err := tw.WriteHeader(&tar.Header{Name: "pack.json", Size: int64(len(b)), Mode: 0o644}); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write(b); err != nil {
					t.Fatal(err)
				}
			}
			if err := tw.WriteHeader(&tc.header); err != nil {
				t.Fatal(err)
			}
			// The oversized payload is intentionally absent: the header must be
			// rejected before attempting to read or allocate it.
			tw.Close()
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := e.App.ImportPack(file, false); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("import: %v", err)
			}
			if _, _, err := e.App.VerifyPackFile(file); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("streaming verify: %v", err)
			}
			assertNoPackTemp(t, e.App.PacksDir(), p.Name)
		})
	}
}
