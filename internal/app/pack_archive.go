package app

import (
	"archive/tar"
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/klauspost/compress/zstd"
)

const packManifestLimit = 64 << 20
const packFileLimit = int64(4 << 30)

// PackProblem identifies a copy that is missing, unreadable or has changed.
type PackProblem struct {
	PackageID, Name, Folder, Why string
}

func packProblem(e PackEntry, why string) PackProblem {
	return PackProblem{PackageID: e.PackageID, Name: e.Name, Folder: e.Folder, Why: why}
}

func packProblemsError(ps []PackProblem) error {
	var lines []string
	for _, p := range ps {
		lines = append(lines, fmt.Sprintf("%s (pkg:%s, mods/%s): %s", p.Name, p.PackageID, p.Folder, p.Why))
	}
	return fmt.Errorf("pack verification failed:\n  %s", strings.Join(lines, "\n  "))
}

type copyFileDigest struct {
	path string
	size int64
	sum  string
}

func digestFiles(files []copyFileDigest) (int, string) {
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%d\x00%s\n", f.path, f.size, f.sum)
	}
	return len(files), hex.EncodeToString(h.Sum(nil))
}

func copyMetadata(rel string) bool {
	return rel == ".rimarcho" || strings.HasPrefix(rel, ".rimarcho/")
}

// copyDigest covers the carried bytes rather than the scan's semantic snapshot.
func copyDigest(dir string) (files int, sum string, err error) {
	fi, err := os.Lstat(dir)
	if err != nil {
		return 0, "", err
	}
	if !fi.IsDir() || source.IsLink(dir) {
		return 0, "", fmt.Errorf("%s: expected a directory, no links allowed", dir)
	}
	var records []copyFileDigest
	err = filepath.WalkDir(dir, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 || (d.Type() != 0 && source.IsLink(file)) {
			return fmt.Errorf("%s: links are not allowed in a copy", file)
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: only directories and regular files are allowed", file)
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if copyMetadata(rel) {
			return nil
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		cerr := f.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
		records = append(records, copyFileDigest{rel, n, hex.EncodeToString(h.Sum(nil))})
		return nil
	})
	if err != nil {
		return 0, "", err
	}
	files, sum = digestFiles(records)
	return files, sum, nil
}

// archivePath deliberately uses slash paths on every host. Reject Windows aliases
// too, so an archive cannot change meaning when it is imported on Windows.
func archivePath(name string) (string, error) {
	n := strings.TrimSuffix(name, "/")
	if n == "" || strings.HasPrefix(n, "/") || strings.ContainsAny(n, "\\:\x00") {
		return "", fmt.Errorf("unsafe path in the pack: %q", name)
	}
	for _, part := range strings.Split(n, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || !filepath.IsLocal(part) {
			return "", fmt.Errorf("unsafe path in the pack: %q", name)
		}
	}
	return n, nil
}

func packFolders(p *Pack) (map[string]PackEntry, error) {
	if p.Format < 1 || p.Format > PackFormat {
		return nil, fmt.Errorf("pack has format %d; this RimArcho reads formats 1–%d", p.Format, PackFormat)
	}
	if err := checkPackName(p.Name); err != nil {
		return nil, err
	}
	folders := map[string]PackEntry{}
	used := map[string]bool{}
	for _, e := range p.Entries {
		if e.Bytes < 0 {
			return nil, fmt.Errorf("%s: negative copy size", e.Name)
		}
		if e.Official {
			if e.Folder != "" {
				return nil, fmt.Errorf("%s: official game content cannot have a folder (%s)", e.Name, e.Folder)
			}
			continue
		}
		if strings.HasPrefix(strings.ToLower(e.PackageID), "ludeon.rimworld") {
			return nil, fmt.Errorf("%s (pkg:%s): game content cannot be carried", e.Name, e.PackageID)
		}
		if e.Folder == "" {
			if p.Light {
				continue
			}
			return nil, fmt.Errorf("%s (pkg:%s): missing carried folder", e.Name, e.PackageID)
		}
		folder, err := archivePath(e.Folder)
		if err != nil || strings.Contains(e.Folder, "/") || used[strings.ToLower(folder)] {
			return nil, fmt.Errorf("%s: invalid or duplicate carried folder %q", e.Name, e.Folder)
		}
		used[strings.ToLower(folder)] = true
		folders[folder] = e
	}
	return folders, nil
}

func digestProblem(e PackEntry, files int, sum string) string {
	if e.Files != files || e.SHA256 != sum {
		return fmt.Sprintf("content differs (files %d, expected %d; sha256 %s, expected %s); rebuild the pack", files, e.Files, sum, e.SHA256)
	}
	return ""
}

func verifyPackDir(p *Pack, dir string) ([]PackProblem, error) {
	folders, err := packFolders(p)
	if err != nil {
		return nil, err
	}
	var problems []PackProblem
	for _, e := range p.Entries {
		if e.Official || e.Folder == "" {
			continue
		}
		n, sum, err := copyDigest(filepath.Join(dir, "mods", e.Folder))
		why := ""
		if err != nil {
			why = err.Error()
		} else if p.Format >= 2 {
			why = digestProblem(e, n, sum)
		}
		if why != "" {
			problems = append(problems, packProblem(e, why))
		}
	}
	ents, err := os.ReadDir(filepath.Join(dir, "mods"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, de := range ents {
		if _, ok := folders[de.Name()]; !ok {
			problems = append(problems, PackProblem{Folder: de.Name(), Why: "path is not in the pack manifest"})
		}
	}
	return problems, nil
}

// VerifyPack checks an installed pack's copies. Game/DLC matching is a separate step.
func (a *App) VerifyPack(name string) ([]PackProblem, error) {
	if err := checkPackName(name); err != nil {
		return nil, err
	}
	p, err := a.ReadPack(name)
	if err != nil {
		return nil, err
	}
	return verifyPackDir(p, a.packPath(name))
}

type packArchiveEntry struct {
	name     string
	dir      bool
	size     int64
	modified time.Time
	r        io.Reader
}

type packArchive struct {
	p        *Pack
	manifest []byte
	next     func() (*packArchiveEntry, error)
	close    func() error
}

func readPackManifest(r io.Reader) ([]byte, *Pack, error) {
	b, err := io.ReadAll(io.LimitReader(r, packManifestLimit+1))
	if err != nil {
		return nil, nil, err
	}
	if len(b) > packManifestLimit {
		return nil, nil, fmt.Errorf("pack.json exceeds 64 MB")
	}
	var p Pack
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, nil, fmt.Errorf("pack.json: %w", err)
	}
	if _, err := packFolders(&p); err != nil {
		return nil, nil, err
	}
	return b, &p, nil
}

func openPackArchive(file string) (*packArchive, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	if string(magic[:]) == "PK\x03\x04" {
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		zr, err := zip.NewReader(f, fi.Size())
		if err != nil {
			f.Close()
			return nil, err
		}
		ar, err := zipPackArchive(zr)
		if err != nil {
			f.Close()
			return nil, err
		}
		closeZip := ar.close
		ar.close = func() error { closeZip(); return f.Close() }
		return ar, nil
	}
	if string(magic[:]) != "\x28\xb5\x2f\xfd" {
		f.Close()
		return nil, fmt.Errorf("%s: not a ZIP or zstd pack", file)
	}
	dec, err := zstd.NewReader(f, zstd.WithDecoderLowmem(true), zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(64<<20))
	if err != nil {
		f.Close()
		return nil, err
	}
	closeArchive := func() error { dec.Close(); return f.Close() }
	tr := tar.NewReader(dec)
	h, err := tr.Next()
	if err != nil {
		closeArchive()
		return nil, fmt.Errorf("pack.json must be the first tar entry: %w", err)
	}
	if h.Name != "pack.json" || h.Typeflag != tar.TypeReg || h.Size > packManifestLimit {
		closeArchive()
		return nil, fmt.Errorf("first tar entry must be regular pack.json (at most 64 MB)")
	}
	b, p, err := readPackManifest(tr)
	if err != nil {
		closeArchive()
		return nil, err
	}
	ar := &packArchive{p: p, manifest: b, close: closeArchive}
	ar.next = func() (*packArchiveEntry, error) {
		h, err := tr.Next()
		if err == io.EOF {
			// Finish the zstd frame too: tar ends before its checksum/trailer.
			if _, e := io.Copy(io.Discard, dec); e != nil {
				return nil, e
			}
			return nil, io.EOF
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir {
			return nil, fmt.Errorf("%s: only directories and regular files are allowed", h.Name)
		}
		return &packArchiveEntry{h.Name, h.Typeflag == tar.TypeDir, h.Size, h.ModTime, tr}, nil
	}
	return ar, nil
}

func zipPackArchive(zr *zip.Reader) (*packArchive, error) {
	var manifest *zip.File
	for _, f := range zr.File {
		if f.Name == "pack.json" {
			if manifest != nil {
				return nil, fmt.Errorf("duplicate pack.json")
			}
			manifest = f
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("the file has no pack.json")
	}
	if !manifest.Mode().IsRegular() || manifest.UncompressedSize64 > packManifestLimit {
		return nil, fmt.Errorf("pack.json must be a regular file of at most 64 MB")
	}
	rc, err := manifest.Open()
	if err != nil {
		return nil, err
	}
	b, p, err := readPackManifest(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}
	i := 0
	var current io.ReadCloser
	ar := &packArchive{p: p, manifest: b, close: func() error {
		if current != nil {
			return current.Close()
		}
		return nil
	}}
	ar.next = func() (*packArchiveEntry, error) {
		if current != nil {
			current.Close()
			current = nil
		}
		for i < len(zr.File) {
			f := zr.File[i]
			i++
			if f == manifest {
				continue
			}
			if !f.Mode().IsRegular() && !f.Mode().IsDir() {
				return nil, fmt.Errorf("%s: only directories and regular files are allowed", f.Name)
			}
			if f.UncompressedSize64 > uint64(packFileLimit) {
				return nil, fmt.Errorf("%s: file exceeds 4 GB", f.Name)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			current = rc
			return &packArchiveEntry{f.Name, f.Mode().IsDir(), int64(f.UncompressedSize64), f.Modified, rc}, nil
		}
		return nil, io.EOF
	}
	return ar, nil
}

// scanPackArchive hashes each file while streaming, retaining only its path, size
// and hash. Import and pre-import verification share the same whitelist.
func scanPackArchive(ar *packArchive, dst string) ([]PackProblem, error) {
	folders, err := packFolders(ar.p)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{"pack.json": true}
	kinds := map[string]bool{"pack.json": false} // includes implicit parent directories
	present := map[string]bool{}
	records := map[string][]copyFileDigest{}
	for {
		e, err := ar.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		n, err := archivePath(e.name)
		if err != nil {
			return nil, err
		}
		parts := strings.SplitN(n, "/", 3)
		if len(parts) < 2 || parts[0] != "mods" {
			return nil, fmt.Errorf("%s: path is not allowed in a pack", e.name)
		}
		if _, ok := folders[parts[1]]; !ok {
			return nil, fmt.Errorf("%s: mod folder is not in pack.json", e.name)
		}
		if len(parts) == 2 && !e.dir {
			return nil, fmt.Errorf("%s: carried folder must be a directory", e.name)
		}
		if e.size < 0 || e.size > packFileLimit {
			return nil, fmt.Errorf("%s: file exceeds 4 GB", e.name)
		}
		if e.dir && e.size != 0 {
			return nil, fmt.Errorf("%s: directory has file content", e.name)
		}
		key := strings.ToLower(n)
		if seen[key] {
			return nil, fmt.Errorf("%s: duplicate archive path", e.name)
		}
		if dir, exists := kinds[key]; exists && (!dir || !e.dir) {
			return nil, fmt.Errorf("%s: conflicting archive path", e.name)
		}
		seen[key], kinds[key] = true, e.dir
		for parent := path.Dir(key); parent != "."; parent = path.Dir(parent) {
			if dir, exists := kinds[parent]; exists && !dir {
				return nil, fmt.Errorf("%s: parent is a file", e.name)
			}
			kinds[parent] = true
		}
		present[parts[1]] = true
		var out *os.File
		if dst != "" {
			file := filepath.Join(dst, filepath.FromSlash(n))
			if !within(dst, file) {
				return nil, fmt.Errorf("unsafe path in the pack: %s", e.name)
			}
			if e.dir {
				err = os.MkdirAll(file, 0o755)
			} else {
				err = os.MkdirAll(filepath.Dir(file), 0o755)
				if err == nil {
					out, err = os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
				}
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.name, err)
			}
		}
		if e.dir {
			continue
		}
		h := sha256.New()
		var w io.Writer = h
		if out != nil {
			w = io.MultiWriter(h, out)
		}
		nbytes, err := io.Copy(w, io.LimitReader(e.r, packFileLimit+1))
		if out != nil {
			if cerr := out.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.name, err)
		}
		if nbytes > packFileLimit || nbytes != e.size {
			return nil, fmt.Errorf("%s: file size differs or exceeds 4 GB", e.name)
		}
		if dst != "" && !e.modified.IsZero() {
			file := filepath.Join(dst, filepath.FromSlash(n))
			if err := os.Chtimes(file, e.modified, e.modified); err != nil {
				return nil, err
			}
		}
		if !copyMetadata(parts[2]) {
			records[parts[1]] = append(records[parts[1]], copyFileDigest{parts[2], nbytes, hex.EncodeToString(h.Sum(nil))})
		}
	}
	var problems []PackProblem
	for _, e := range ar.p.Entries {
		if e.Official || e.Folder == "" {
			continue
		}
		why := ""
		if !present[e.Folder] {
			why = "carried folder is missing"
		} else if ar.p.Format >= 2 {
			n, sum := digestFiles(records[e.Folder])
			why = digestProblem(e, n, sum)
		}
		if why != "" {
			problems = append(problems, packProblem(e, why))
		}
	}
	return problems, nil
}

// VerifyPackFile verifies the archive without extracting it or changing the store.
func (a *App) VerifyPackFile(file string) (*Pack, []PackProblem, error) {
	ar, err := openPackArchive(file)
	if err != nil {
		return nil, nil, err
	}
	defer ar.close()
	problems, err := scanPackArchive(ar, "")
	return ar.p, problems, err
}

func (a *App) importPackArchive(file string) (*PackMatch, error) {
	ar, err := openPackArchive(file)
	if err != nil {
		return nil, err
	}
	defer ar.close()
	return a.extractPackArchive(ar)
}

// unpackPack retains the format-1 reader for callers holding an open ZIP reader.
func (a *App) unpackPack(zr *zip.Reader) (*PackMatch, error) {
	ar, err := zipPackArchive(zr)
	if err != nil {
		return nil, err
	}
	defer ar.close()
	return a.extractPackArchive(ar)
}

func (a *App) extractPackArchive(ar *packArchive) (*PackMatch, error) {
	p := ar.p
	dir := a.packPath(p.Name)
	if _, err := os.Lstat(dir); err == nil {
		return nil, fmt.Errorf("pack %q exists; remove it first", p.Name)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	tmp := dir + TmpSuffix
	if b := ReadBuildProgress(tmp); b != nil && b.Running() {
		return nil, fmt.Errorf("%w (%s)", ErrBuildRunning, p.Name)
	}
	need := int64(SpaceMargin)
	for _, e := range p.Entries {
		if e.Bytes > math.MaxInt64-need {
			return nil, fmt.Errorf("pack copy sizes overflow")
		}
		need += e.Bytes
	}
	free := a.FreeSpace
	if free == nil {
		free = DiskFree
	}
	if available := free(a.PacksDir()); available >= 0 && available < need {
		return nil, fmt.Errorf("%w: needs %d bytes, %d free", ErrNoSpace, need, available)
	}
	if err := a.managedRootReady("packs", a.PacksDir()); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(tmp); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "pack.json"), ar.manifest, 0o644); err != nil {
		return nil, err
	}
	problems, err := scanPackArchive(ar, tmp)
	if err != nil {
		return nil, err
	}
	// the stream was hashed as it was written, and the whitelist allows nothing else
	// under mods/, so the extracted tree is not hashed a second time
	if len(problems) > 0 {
		return nil, packProblemsError(problems)
	}
	if err := a.Store.SetMeta(directoryKey("packs"), a.PacksDir()); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return nil, err
	}
	return &PackMatch{Pack: p, Dir: dir, Verified: p.Format >= 2}, nil
}

func (a *App) exportPackArchive(name, file string, p *Pack) (int64, error) {
	if _, err := packFolders(p); err != nil {
		return 0, err
	}
	root := a.packPath(name)
	if p.Format == 1 {
		var problems []PackProblem
		for i := range p.Entries {
			e := &p.Entries[i]
			if e.Official || e.Folder == "" {
				continue
			}
			n, sum, err := copyDigest(filepath.Join(root, "mods", e.Folder))
			if err != nil {
				problems = append(problems, packProblem(*e, err.Error()))
				continue
			}
			e.Files, e.SHA256 = n, sum
		}
		if len(problems) > 0 {
			return 0, packProblemsError(problems)
		}
		p.Format = PackFormat
		b, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return 0, err
		}
		manifest := filepath.Join(root, "pack.json")
		out, err := os.CreateTemp(root, "pack.json-*.part")
		if err != nil {
			return 0, err
		}
		defer os.Remove(out.Name())
		_, err = out.Write(append(b, '\n'))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return 0, err
		}
		if err := os.Rename(out.Name(), manifest); err != nil {
			return 0, err
		}
	}
	problems, err := verifyPackDir(p, root)
	if err != nil {
		return 0, err
	}
	if len(problems) > 0 {
		return 0, packProblemsError(problems)
	}
	f, err := os.OpenFile(file+".part", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer os.Remove(file + ".part")
	zw, err := zstd.NewWriter(f, zstd.WithEncoderLevel(zstd.SpeedBetterCompression), zstd.WithWindowSize(8<<20), zstd.WithEncoderConcurrency(1))
	if err != nil {
		f.Close()
		return 0, err
	}
	tw := tar.NewWriter(zw)
	writeFile := func(file, rel string, fi fs.FileInfo) error {
		if !fi.IsDir() && !fi.Mode().IsRegular() {
			return fmt.Errorf("%s: only directories and regular files are allowed", rel)
		}
		if fi.Size() > packFileLimit && !fi.IsDir() {
			return fmt.Errorf("%s: file exceeds 4 GB", rel)
		}
		if _, err := archivePath(rel); err != nil {
			return err
		}
		h, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		h.Name, h.Format = rel, tar.FormatPAX
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		in, err := os.Open(file)
		if err != nil {
			return err
		}
		_, err = io.Copy(tw, in)
		cerr := in.Close()
		if err != nil {
			return err
		}
		return cerr
	}
	manifest := filepath.Join(root, "pack.json")
	fi, err := os.Lstat(manifest)
	if err == nil {
		err = writeFile(manifest, "pack.json", fi)
	}
	var folders []string
	for _, e := range p.Entries {
		if !e.Official && e.Folder != "" {
			folders = append(folders, e.Folder)
		}
	}
	sort.Strings(folders)
	for _, folder := range folders {
		if err != nil {
			break
		}
		modRoot := filepath.Join(root, "mods", folder)
		err = filepath.WalkDir(modRoot, func(file string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return err
			}
			return writeFile(file, filepath.ToSlash(rel), fi)
		})
	}
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := zw.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	if _, err := os.Lstat(file); err == nil {
		return 0, fmt.Errorf("%s exists; nothing is overwritten", file)
	} else if !os.IsNotExist(err) {
		return 0, err
	}
	if err := os.Rename(file+".part", file); err != nil {
		return 0, err
	}
	fi, err = os.Stat(file)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
