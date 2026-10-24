package subscribe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/source"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	"github.com/go-git/go-git/v5/storage/memory"
)

var commitRE = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// NormalizeRepo compares repository identities without changing their transport URL.
func NormalizeRepo(raw string) string {
	p, err := repositoryURL(raw)
	if err != nil {
		return raw
	}
	u, err := url.Parse(p)
	if err == nil && strings.EqualFold(u.Scheme, "https") {
		u.Scheme, u.Host = "https", strings.ToLower(u.Host)
		u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), ".git")
		u.RawPath, u.RawQuery, u.Fragment = "", "", ""
		return u.String()
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	// The same folder by a link or an 8.3 short name is the same repository.
	if real, err := source.RealPath(p); err == nil {
		p = real
	}
	return env.PathKey(p)
}

func init() {
	// go-git's default file transport starts git-upload-pack from PATH.
	client.InstallProtocol("file", newLocalTransport())
}

// Validate accepts public HTTPS repositories and local repositories only.
func Validate(rawURL, refType, ref string) error {
	if _, err := repositoryURL(rawURL); err != nil {
		return err
	}
	switch refType {
	case "branch", "tag":
		if ref == "" && refType == "branch" {
			return nil
		}
		name := plumbing.NewBranchReferenceName(ref)
		if refType == "tag" {
			name = plumbing.NewTagReferenceName(ref)
		}
		if ref == "" || name.Validate() != nil {
			return fmt.Errorf("invalid %s name %q", refType, ref)
		}
	case "commit":
		if !commitRE.MatchString(ref) {
			return fmt.Errorf("commit must be a full 40-character SHA: %q", ref)
		}
	default:
		return fmt.Errorf("unknown Git ref type %q; use branch, tag or commit", refType)
	}
	return nil
}

func repositoryURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("no Git repository given")
	}
	if filepath.IsAbs(raw) {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.User != nil {
		return "", fmt.Errorf("Git subscriptions support repositories without login only")
	}
	switch u.Scheme {
	case "https":
		if u.Host == "" {
			return "", fmt.Errorf("invalid repository URL %q", raw)
		}
		return raw, nil
	case "file":
		p := u.Path
		if u.Host != "" && u.Host != "localhost" {
			p = "//" + u.Host + p
		}
		if len(p) > 3 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
		return filepath.FromSlash(p), nil
	case "":
		if strings.Contains(raw, ":") {
			return "", fmt.Errorf("use an HTTPS URL, file URL or local path")
		}
		return raw, nil
	default:
		return "", fmt.Errorf("unsupported Git URL %q; use HTTPS, file:// or a local path", raw)
	}
}

func Clone(ctx context.Context, rawURL, refType, ref, dir string) (string, error) {
	if err := Validate(rawURL, refType, ref); err != nil {
		return "", err
	}
	u, _ := repositoryURL(rawURL)
	o := &git.CloneOptions{URL: u, Tags: git.NoTags}
	if refType == "commit" {
		o.Tags = git.AllTags
	}
	if refType != "commit" {
		o.Depth, o.SingleBranch = 1, true
		if ref != "" {
			o.ReferenceName = plumbing.NewBranchReferenceName(ref)
			if refType == "tag" {
				o.ReferenceName = plumbing.NewTagReferenceName(ref)
			}
		}
	}
	r, err := git.PlainCloneContext(ctx, dir, false, o)
	if err != nil {
		return "", err
	}
	if refType == "commit" {
		w, err := r.Worktree()
		if err != nil {
			return "", err
		}
		if err := w.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(ref)}); err != nil {
			return "", err
		}
	}
	if err := excludeMetadata(dir); err != nil {
		return "", err
	}
	head, err := r.Head()
	if err != nil {
		return "", err
	}
	return head.Hash().String(), nil
}

func excludeMetadata(dir string) error {
	p := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString("\n.rimarcho/\n")
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func RemoteCommit(ctx context.Context, rawURL, refType, ref string) (string, error) {
	if err := Validate(rawURL, refType, ref); err != nil {
		return "", err
	}
	if refType == "commit" {
		return ref, nil
	}
	u, _ := repositoryURL(rawURL)
	r := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{u}})
	refs, err := r.ListContext(ctx, &git.ListOptions{PeelingOption: git.AppendPeeled})
	if err != nil {
		return "", err
	}
	name := plumbing.HEAD
	if refType == "tag" {
		name = plumbing.NewTagReferenceName(ref)
	} else if ref != "" {
		name = plumbing.NewBranchReferenceName(ref)
	}
	byName := map[plumbing.ReferenceName]*plumbing.Reference{}
	for _, r := range refs {
		byName[r.Name()] = r
	}
	if refType == "tag" {
		if r := byName[plumbing.ReferenceName(name.String()+"^{}")]; r != nil {
			return r.Hash().String(), nil
		}
	}
	for n := 0; n < len(refs); n++ {
		r := byName[name]
		if r == nil {
			break
		}
		if r.Type() == plumbing.HashReference {
			return r.Hash().String(), nil
		}
		name = r.Target()
	}
	return "", fmt.Errorf("repository %s has no %s %q", rawURL, refType, ref)
}

func Dirty(dir string) (bool, error) {
	r, err := git.PlainOpen(dir)
	if err != nil {
		return false, err
	}
	w, err := r.Worktree()
	if err != nil {
		return false, err
	}
	s, err := w.Status()
	if err != nil {
		return false, err
	}
	for path, status := range s {
		// go-billy on Windows cannot read .git/info/exclude's slash-separated
		// internal path. Keep metadata out of our status even on that platform.
		if path == ".rimarcho" || strings.HasPrefix(filepath.ToSlash(path), ".rimarcho/") {
			continue
		}
		if status.Staging == git.Untracked {
			continue
		}
		if status.Staging != git.Unmodified || status.Worktree != git.Unmodified {
			return true, nil
		}
	}
	return false, nil
}

// Update refuses local edits before fetching or replacing any worktree files.
func Update(ctx context.Context, dir string, rec *Record) (string, error) {
	dirty, err := Dirty(dir)
	if err != nil {
		return "", err
	}
	if dirty {
		return "", fmt.Errorf("dirty Git worktree at %s; save or discard local changes before updating", dir)
	}
	commit, err := RemoteCommit(ctx, rec.GitURL(), rec.RefType, rec.Ref)
	if err != nil {
		return "", err
	}
	r, err := git.PlainOpen(dir)
	if err != nil {
		return "", err
	}
	u, _ := repositoryURL(rec.GitURL())
	depth := 1
	ref := rec.Ref
	if rec.RefType == "branch" && ref == "" {
		remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{u}})
		refs, err := remote.ListContext(ctx, &git.ListOptions{})
		if err != nil {
			return "", err
		}
		for _, r := range refs {
			if r.Name() == plumbing.HEAD && r.Type() == plumbing.SymbolicReference {
				ref = r.Target().Short()
			}
		}
		if ref == "" {
			return "", fmt.Errorf("remote HEAD does not name a branch")
		}
	}
	specs := []config.RefSpec{config.RefSpec("+refs/heads/" + ref + ":refs/remotes/origin/" + ref)}
	if rec.RefType == "tag" {
		specs = []config.RefSpec{config.RefSpec("+refs/tags/" + ref + ":refs/tags/" + ref)}
	}
	if rec.RefType == "commit" {
		depth = 0
		specs = []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*"}
	}
	err = r.FetchContext(ctx, &git.FetchOptions{RemoteURL: u, Force: true, Depth: depth, Tags: git.NoTags,
		RefSpecs: specs})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) && !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return "", err
	}
	w, err := r.Worktree()
	if err != nil {
		return "", err
	}
	if err := protectUntracked(r, dir, plumbing.NewHash(commit)); err != nil {
		return "", err
	}
	// The worktree is clean apart from untracked files. Merge checkout limits
	// replacement to tracked paths; go-git's Force checkout deletes untracked files.
	if err := w.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(commit)}); err != nil {
		return "", err
	}
	return commit, nil
}

// A clean worktree can still contain generated textures. Refuse a checkout that
// would overwrite any of those paths (or an untracked parent file).
func protectUntracked(r *git.Repository, dir string, hash plumbing.Hash) error {
	dir = filepath.Clean(dir)
	idx, err := r.Storer.Index()
	if err != nil {
		return err
	}
	tracked := make(map[string]bool, len(idx.Entries))
	for _, e := range idx.Entries {
		tracked[env.PathKey(filepath.Join(dir, filepath.FromSlash(e.Name)))] = true
	}
	commit, err := r.CommitObject(hash)
	if err != nil {
		return err
	}
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	files := tree.Files()
	defer files.Close()
	for {
		file, err := files.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		path := filepath.Join(dir, filepath.FromSlash(file.Name))
		for p := path; p != dir; p = filepath.Dir(p) {
			if tracked[env.PathKey(p)] {
				break
			}
			st, err := os.Lstat(p)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			if p == path || !st.IsDir() {
				return fmt.Errorf("update would overwrite untracked path %s; move it before updating", p)
			}
		}
	}
}

// FolderName derives a portable directory name, including for Windows local paths.
func FolderName(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && len(u.Scheme) > 1 {
		raw = u.Path
	}
	raw = strings.TrimRight(strings.ReplaceAll(raw, "\\", "/"), "/")
	if i := strings.LastIndexByte(raw, '/'); i >= 0 {
		raw = raw[i+1:]
	}
	raw = strings.TrimSuffix(raw, ".git")
	raw = strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '-'
		}
		return r
	}, raw)
	raw = strings.Trim(raw, " .")
	base := strings.ToUpper(strings.SplitN(raw, ".", 2)[0])
	if raw == "" || strings.HasPrefix(raw, ".archo-") {
		return "repository"
	}
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || regexp.MustCompile(`^(COM|LPT)[1-9]$`).MatchString(base) {
		raw = "_" + raw
	}
	return raw
}
