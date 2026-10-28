package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/render"
)

func problemMuteVersionCmd(c *Ctx, a *Args) (*render.Doc, error) { return versionMuteCmd(c, a, true) }
func problemUnmuteVersionCmd(c *Ctx, a *Args) (*render.Doc, error) {
	return versionMuteCmd(c, a, false)
}

func versionMuteCmd(c *Ctx, a *Args, muted bool) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	mm := st.GameMM()
	if err := c.App.SetVersionMute(a.Pos, mm, muted); err != nil {
		return nil, err
	}
	verb := "ignored"
	if !muted {
		verb = "restored"
	}
	d, root := render.New(fmt.Sprintf("version warnings %s for %s", verb, mm))
	root.KV("mods", strings.Join(a.Pos, " "))
	root.KV("scope", "packages in every plan and environment; survives mod updates; other game minor versions are unchanged")
	return d, nil
}
