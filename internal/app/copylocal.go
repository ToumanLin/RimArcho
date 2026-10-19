package app

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// CopyResult describes a finished copy-local (04 §6).
type CopyResult struct {
	Dir      string // the new folder under the local source
	Files    int
	Bytes    int64
	Skipped  []string // links that point outside the mod folder, relative to it
	GitError string   // --git failed after the copy succeeded
	Git      bool
}

// LocalSource is the environment's local source (the game's Mods folder).
func (ix *Index) LocalSource() *source.Source {
	for _, s := range ix.Sources {
		if s.Kind == source.Local {
			return s
		}
	}
	return nil
}

// CopyLocal copies an instance into the local source with an origin marker (04 §6). It
// never changes the Loadout, never overwrites an existing folder, never copies .git and
// never follows links that leave the mod folder. The copy is assembled next to its final
// place and renamed, so a failure leaves nothing behind.
func (a *App) CopyLocal(st *LoadoutState, m *store.Mod, name string, git bool) (*CopyResult, error) {
	ix := st.Ix
	ls := ix.LocalSource()
	if ls == nil {
		return nil, fmt.Errorf("the environment has no local source (the game's Mods folder)")
	}
	src := ix.ModDir(m)
	if real, err := source.RealPath(src); err == nil {
		src = real
	}
	if name == "" {
		name = filepath.Base(m.Rel) + "_local"
	}
	if name != filepath.Base(name) || name == "." || name == ".." || strings.ContainsAny(name, `<>:"|?*`) {
		return nil, fmt.Errorf("--name must be a plain folder name: %q", name)
	}
	dst := filepath.Join(ls.Root, name)
	if _, err := os.Lstat(dst); err == nil {
		return nil, fmt.Errorf("%s already exists; choose another --name (nothing is overwritten)", dst)
	}
	tmp := dst + ".archo-tmp"
	os.RemoveAll(tmp)
	res := &CopyResult{Dir: dst}
	if err := copyTree(src, src, tmp, "", res); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	o := scan.Origin{InstanceID: m.ID, LineageID: m.LineageID, WorkshopID: m.WorkshopID(), SnapshotHash: hex.EncodeToString(m.SnapHash),
		CopiedAt: a.Now().UTC().Format("2006-01-02T15:04:05Z")}
	b, _ := json.MarshalIndent(o, "", "  ")
	if err := os.MkdirAll(filepath.Join(tmp, ".rimarcho"), 0o755); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(tmp, filepath.FromSlash(scan.OriginFile)), append(b, '\n'), 0o644); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if git {
		if err := gitBaseline(dst, m.DisplayName(), m.ID); err != nil {
			res.GitError = err.Error()
		} else {
			res.Git = true
		}
	}
	return res, nil
}

// copyTree copies dir (whose path relative to the mod root is rel) into dst.
func copyTree(root, dir, dst, rel string, res *CopyResult) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, de := range ents {
		p := filepath.Join(dir, de.Name())
		r := filepath.ToSlash(filepath.Join(rel, de.Name()))
		if strings.EqualFold(de.Name(), ".git") {
			continue // the copy gets its own history (--git), never the upstream one
		}
		if rel == "" && strings.EqualFold(de.Name(), ".rimarcho") {
			continue // a fresh origin marker is written
		}
		target := p
		if source.IsLink(p) {
			t, err := source.RealPath(p)
			if err != nil || !within(root, t) || within(t, dir) {
				res.Skipped = append(res.Skipped, r) // outside the mod folder, or a loop
				continue
			}
			target = t
		}
		fi, err := os.Stat(target)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			if err := copyTree(root, target, filepath.Join(dst, de.Name()), r, res); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(target, filepath.Join(dst, de.Name()), fi); err != nil {
			return err
		}
		res.Files++
		res.Bytes += fi.Size()
	}
	return nil
}

// within reports whether p is root or below it (case-insensitive, as on Windows).
func within(root, p string) bool {
	r := strings.ToLower(filepath.Clean(root))
	q := strings.ToLower(filepath.Clean(p))
	return q == r || strings.HasPrefix(q, r+string(os.PathSeparator))
}

func copyFile(src, dst string, fi os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, fi.ModTime(), fi.ModTime())
}

// gitBaseline makes the copy a repository with one commit of everything, ignored files
// included (the baseline must be the exact copy).
func gitBaseline(dir, name, origin string) error {
	run := func(args ...string) error {
		cmd := exec.Command("git", append([]string{"-c", "core.autocrlf=false", "-c", "user.name=RimArcho", "-c", "user.email=rimarcho@localhost", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found on PATH; the copy was made without a repository")
	}
	if err := run("init", "-q"); err != nil {
		return err
	}
	if err := run("add", "--all", "--force", "."); err != nil {
		return err
	}
	return run("commit", "-q", "--no-verify", "-m", fmt.Sprintf("Baseline: copy of %s (#%s)", name, origin))
}
