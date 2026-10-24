package testkit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// GitRepo is a local worktree and bare upstream. No Git executable or network is used.
type GitRepo struct {
	Work, Bare string
	Repo       *git.Repository
	First      string
}

func NewGitRepo(t testing.TB, subdir string) *GitRepo {
	t.Helper()
	t.Setenv("PATH", "")
	root := t.TempDir()
	g := &GitRepo{Work: filepath.Join(root, "work"), Bare: filepath.Join(root, "mod.git")}
	var err error
	g.Repo, err = git.PlainInitWithOptions(g.Work, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName("main")}})
	if err != nil {
		t.Fatal(err)
	}
	g.First = g.Commit(t, filepath.Join(subdir, "About", "About.xml"), `<ModMetaData><name>Git Mod</name><packageId>tester.gitmod</packageId><supportedVersions><li>1.6</li></supportedVersions></ModMetaData>`)
	if _, err := g.Repo.CreateTag("v1", plumbing.NewHash(g.First), &git.CreateTagOptions{Tagger: &object.Signature{Name: "Tester", Email: "test@example.invalid", When: Start}, Message: "annotated release"}); err != nil {
		t.Fatal(err)
	}
	if _, err := git.PlainClone(g.Bare, true, &git.CloneOptions{URL: g.Work}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{g.Bare}}); err != nil {
		t.Fatal(err)
	}
	return g
}

func (g *GitRepo) Commit(t testing.TB, path, content string) string {
	t.Helper()
	p := filepath.Join(g.Work, path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := g.Repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add(filepath.ToSlash(path)); err != nil {
		t.Fatal(err)
	}
	h, err := w.Commit("update "+path, &git.CommitOptions{Author: &object.Signature{Name: "Tester", Email: "test@example.invalid", When: Start.Add(time.Minute)}})
	if err != nil {
		t.Fatal(err)
	}
	if g.First != "" {
		if err := g.Repo.Push(&git.PushOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return h.String()
}
