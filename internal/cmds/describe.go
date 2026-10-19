package cmds

import (
	"fmt"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// describe (06 §6.4): the mod's description from About.xml and the Steam Workshop page.

func init() {
	register(&Command{Name: "describe", Args: "<ref>", Summary: "a mod's description: About.xml and the Workshop page (fetched on demand, cached)", MinArgs: 1, MaxArgs: 1,
		Flags: []Flag{
			{Name: "source", Value: "auto|about|workshop|both", Help: "which text to show (auto: 06 §6.4)"},
			{Name: "refresh", Help: "fetch the Workshop page again"},
			{Name: "offline", Help: "use the cache only"},
		}, Run: describeCmd})
}

func describeCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	src := a.Str("source")
	switch src {
	case "", "auto", "about", "workshop", "both":
	default:
		return nil, &UsageError{Msg: "--source must be auto, about, workshop or both"}
	}
	d := c.App.Describe(ix, m, !a.Bool("offline"), a.Bool("refresh"))
	show := d.Show
	if src != "" && src != "auto" {
		show = src
	}
	doc, root := render.New(fmt.Sprintf("description of %s %s", ix.Ref(m.ID), m.DisplayName()))
	switch {
	case m.WorkshopID() == "":
		root.KV("workshop", "not a workshop mod")
	case d.WorkshopState == "fetched" || d.WorkshopState == "cached":
		root.KV("workshop", fmt.Sprintf("%s (%s, updated %s)", m.WorkshopID(), d.WorkshopState, c.App.FormatDate(d.Updated)))
	default:
		root.KV("workshop", fmt.Sprintf("%s (%s)", m.WorkshopID(), d.WorkshopState))
	}
	add := func(title, text string) {
		n := root.Section(title)
		body := strings.TrimRight(bbcode.Plain(bbcode.Parse(text)), "\n")
		if strings.TrimSpace(body) == "" {
			n.Add("(empty)")
			return
		}
		for _, l := range strings.Split(body, "\n") {
			n.Add(l)
		}
	}
	if show == "about" || show == "both" {
		add("About.xml", d.About)
	}
	if show == "workshop" || show == "both" {
		add("Workshop page", d.Workshop)
	}
	if d.WorkshopState == "offline" {
		doc.AddNext("archo config set workshop.fetch on")
	}
	return doc, nil
}
