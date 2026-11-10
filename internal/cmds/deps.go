package cmds

import (
	"fmt"
	"slices"

	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

func init() {
	register(&Command{Name: "deps", Summary: "show external dependencies: status, version, path and source", MaxArgs: 0, Run: depsCmd})
	installSummary := "download todds or steamcmd into RimArcho's own folder"
	if textures.ToddsZipURL == "" {
		installSummary = "download steamcmd into RimArcho's own folder (provide todds separately)"
	}
	register(&Command{Name: "deps install", Args: "<name>", Summary: installSummary, MinArgs: 1, MaxArgs: 1, Run: depsInstallCmd})
	register(&Command{Name: "deps remove", Args: "<name>", Summary: "remove only RimArcho's own downloaded dependency", MinArgs: 1, MaxArgs: 1, Run: depsRemoveCmd})
}

func depsCmd(c *Ctx, _ *Args) (*render.Doc, error) {
	d, root := render.New("external dependencies")
	for _, i := range c.App.Deps().All() {
		s := root.Section(i.Name)
		status := "not installed"
		if i.OK {
			status = "installed"
		} else if i.Path != "" {
			status = "unavailable"
		}
		s.KV("status", status)
		s.KV("version", i.Version)
		s.KV("path", i.Path)
		s.KV("source", i.Source)
		if u := c.App.ToolUpdate(i.Name); !u.Checked.IsZero() {
			s.KV("last-checked", c.App.FormatTime(u.Checked))
			s.KV("latest", u.Latest)
			s.KV("update-available", fmt.Sprint(u.Pending))
			if !u.Compatible && u.Pending {
				s.KV("update", "new version not yet supported by RimArcho")
			}
		}
		if i.Err != "" {
			s.KV("error", i.Err)
		}
	}
	return d, nil
}

func depName(a *Args) (string, error) {
	n := a.Pos[0]
	if !slices.Contains(deps.Names, n) {
		return "", &UsageError{Msg: fmt.Sprintf("unknown dependency %q (known: %v)", n, deps.Names)}
	}
	return n, nil
}

func depsInstallCmd(c *Ctx, a *Args) (*render.Doc, error) {
	n, err := depName(a)
	if err != nil {
		return nil, err
	}
	p, err := c.App.InstallDep(n, nil)
	if err != nil {
		return nil, err
	}
	d, root := render.New("installed " + n)
	root.KV("path", p)
	return d, nil
}

func depsRemoveCmd(c *Ctx, a *Args) (*render.Doc, error) {
	n, err := depName(a)
	if err != nil {
		return nil, err
	}
	if err := c.App.RemoveDep(n); err != nil {
		return nil, err
	}
	d, _ := render.New("removed " + n)
	return d, nil
}
