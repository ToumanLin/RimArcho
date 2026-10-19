package cmds

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/source"
)

func init() {
	register(&Command{Name: "copy-local", Args: "<ref>", Summary: "copy a mod into the local Mods folder with an origin marker (the Loadout is not changed)", MinArgs: 1, MaxArgs: 1, Run: copyLocalCmd,
		Flags: []Flag{{Name: "name", Value: "<folder>", Help: "default <folder>_local"}, {Name: "git", Help: "make the copy a git repository with a baseline commit"}}})
}

func copyLocalCmd(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	m, err := st.Ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	res, err := c.App.CopyLocal(st, m, a.Str("name"), a.Bool("git"))
	if err != nil {
		return nil, err
	}
	// Scan so the copy has a ref (and its lineage via the origin marker) right away.
	if _, err := c.App.Scan(false); err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	copyRef := ""
	want := res.Dir
	if real, err := source.RealPath(want); err == nil {
		want = real
	}
	for _, x := range ix.Mods {
		if x.TombstoneAt.IsZero() && (strings.EqualFold(filepath.Clean(ix.ModDir(x)), filepath.Clean(want)) || strings.EqualFold(filepath.Clean(ix.ModDir(x)), filepath.Clean(res.Dir))) {
			copyRef = ix.Ref(x.ID)
		}
	}
	d, root := render.New(fmt.Sprintf("%s copied to %s", modLabel(ix, m.ID), res.Dir))
	if copyRef != "" {
		root.KV("copy", copyRef+"  (same lineage, origin marker .rimarcho/origin.json)")
	}
	root.KV("files", fmt.Sprintf("%d (%.1f MB)", res.Files, float64(res.Bytes)/(1<<20)))
	switch {
	case res.Git:
		root.KV("git", "repository with a baseline commit of every file")
	case res.GitError != "":
		root.KV("git", "not created: "+res.GitError)
	}
	if len(res.Skipped) > 0 {
		n := root.KV("skipped", fmt.Sprintf("%d links that point outside the mod folder", len(res.Skipped)))
		for _, s := range res.Skipped {
			n.Add(s)
		}
		n.Truncate(5, "")
	}
	root.KV("loadout", "unchanged")
	if copyRef != "" {
		d.AddNext(fmt.Sprintf("archo swap %s %s", ix.Ref(m.ID), copyRef))
	}
	return d, nil
}
