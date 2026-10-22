package cmds

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
	"github.com/ToumanLin/RimArcho/internal/textures"
)

// 贴图优化 (08 §5.2): the DDS of mods' PNGs, written next to them. The game loads the DDS in
// their place; the PNGs stay. Packs convert their own copies (pack build --textures).
func init() {
	// an existing todds (a folder or the program, e.g. RimSort's); "" is RimArcho's own copy
	freeKeys["textures.todds"] = struct {
		def   string
		check func(string) error
	}{"", func(v string) error {
		if v == "" {
			return nil
		}
		if !filepath.IsAbs(v) {
			return fmt.Errorf("textures.todds must be an absolute path")
		}
		if textures.FindTodds(v) == "" {
			return fmt.Errorf("no todds program at %s", v)
		}
		return nil
	}}
	dry := Flag{Name: "dry-run", Help: "show what would happen"}
	scope := []Flag{{Name: "plan", Help: "the current plan's mods"}, {Name: "all", Help: "every installed mod"}}
	scale := Flag{Name: "scale", Value: "<percent>", Help: "scale textures down (20 25 40 50 60 75 80); a texture never goes below 128 px or off a multiple of 4, masks (_m) stay full size"}
	register(&Command{Name: "textures status", Args: "[<mod>...]", Summary: "PNG textures, the DDS RimArcho made (and stale ones), and what converting them would take and save", MaxArgs: -1, Run: texStatusCmd,
		Flags: append(slices.Clone(scope), scale)})
	register(&Command{Name: "textures optimize", Args: "[<mod>...]", Summary: "write the DDS of mods' PNG textures next to them (the game loads them instead; the PNGs stay)", MaxArgs: -1, Run: texOptimizeCmd,
		Flags: append(slices.Clone(scope), scale,
			Flag{Name: "force", Help: "convert again the textures that are up to date"},
			Flag{Name: "no-mipmaps", Help: "no mipmaps (not recommended: textures shimmer when zoomed out)"}, dry)})
	register(&Command{Name: "textures clean", Args: "[<mod>...]", Summary: "delete the DDS RimArcho wrote (only those; the game goes back to the PNGs)", MaxArgs: -1, Run: texCleanCmd,
		Flags: append(slices.Clone(scope), dry)})
	register(&Command{Name: "textures tool", Summary: "the DDS encoder (todds): where it is, or --install to download it", MaxArgs: 0, Run: texToolCmd,
		Flags: []Flag{{Name: "install", Help: "download todds " + textures.ToddsVersion + " from its GitHub release into RimArcho's own folder"}}})
}

// textureOptions reads --scale (or --texture-scale) and --no-mipmaps.
func textureOptions(a *Args) (textures.Options, error) {
	var o textures.Options
	s := a.Str("scale")
	if s == "" {
		s = a.Str("texture-scale")
	}
	if s != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "%"))
		if err != nil || !slices.Contains(textures.ScaleSteps, n) {
			return o, &UsageError{Msg: fmt.Sprintf("scale %q: one of %s (percent)", s, joinInts(textures.ScaleSteps))}
		}
		o.Scale = n
	}
	o.NoMips = a.Bool("no-mipmaps")
	return o.Normal(), nil
}

func joinInts(ns []int) string {
	s := make([]string, len(ns))
	for i, n := range ns {
		s[i] = strconv.Itoa(n)
	}
	return strings.Join(s, " ")
}

func texScope(a *Args) (string, error) {
	switch {
	case a.Bool("plan") && a.Bool("all"):
		return "", &UsageError{Msg: "--plan or --all, not both"}
	case a.Bool("all"):
		return app.TexAll, nil
	case a.Bool("plan"):
		return app.TexPlan, nil
	case len(a.Pos) == 0:
		return "", &UsageError{Msg: "name the mods, or use --plan or --all"}
	}
	return app.TexRefs, nil
}

// TextureProgressLine is a run's progress line; ParseTextureProgress reads it back (the
// window runs `textures optimize` as a child and follows these lines).
func TextureProgressLine(p app.TextureProgress) string {
	return fmt.Sprintf("progress: textures %d/%d · mods %d/%d · bytes %d · %s", p.Files, p.FilesTotal, p.Mods, p.ModsTotal, p.Bytes, p.Mod)
}

var texProgressRe = regexp.MustCompile(`^progress: textures (\d+)/(\d+) · mods (\d+)/(\d+) · bytes (\d+) · (.*)$`)

// ParseTextureProgress reads a progress line (ok false: not one).
func ParseTextureProgress(l string) (app.TextureProgress, bool) {
	m := texProgressRe.FindStringSubmatch(strings.TrimSpace(l))
	if m == nil {
		return app.TextureProgress{}, false
	}
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	b, _ := strconv.ParseInt(m[5], 10, 64)
	return app.TextureProgress{Files: n(m[1]), FilesTotal: n(m[2]), Mods: n(m[3]), ModsTotal: n(m[4]), Bytes: b, Mod: m[6]}, true
}

// TextureEstimateLine is an estimate in one line (also the interfaces' wording, in English).
func TextureEstimateLine(e textures.Estimate) string {
	return fmt.Sprintf("%d PNGs (%s) → about %s of DDS; video memory about %s → %s", e.Files, mb(e.PNGBytes), mb(e.DDSBytes), mb(e.VRAMBefore), mb(e.VRAMAfter))
}

func texStatusCmd(c *Ctx, a *Args) (*render.Doc, error) {
	sc, err := texScope(a)
	if err != nil {
		return nil, err
	}
	o, err := textureOptions(a)
	if err != nil {
		return nil, err
	}
	r, err := c.App.TextureStatus(a.Pos, sc, o)
	if err != nil {
		return nil, err
	}
	png, pngBytes, ours, oursBytes, stale := r.Totals()
	d, root := render.New(fmt.Sprintf("textures of %d mods: %d PNGs (%s); %d DDS made by RimArcho (%s)", len(r.Mods), png, mb(pngBytes), ours, mb(oursBytes)))
	texToolLine(root, r.Tool)
	root.KV(app.TextureLabel(o), TextureEstimateLine(r.Est))
	if stale > 0 {
		root.KV("stale", fmt.Sprintf("%d DDS whose PNG changed or is gone: the game may show the old texture", stale))
	}
	n := root.Add("mods:")
	for _, m := range r.Mods {
		if m.PNG == 0 && m.Ours == 0 && m.Foreign == 0 {
			continue
		}
		line := fmt.Sprintf("%s %s: %d PNGs (%s)", m.Ref, m.Mod.DisplayName(), m.PNG, mb(m.PNGBytes))
		if m.Ours > 0 {
			line += fmt.Sprintf(", %d DDS by RimArcho (%s)", m.Ours, mb(m.OursSize))
		}
		if m.Stale+m.Orphans > 0 {
			line += fmt.Sprintf(", %d stale", m.Stale+m.Orphans)
		}
		if m.Foreign > 0 {
			line += fmt.Sprintf(", %d DDS of its own", m.Foreign)
		}
		if m.Plan != nil && m.Plan.Est.Files > 0 {
			line += fmt.Sprintf("; %d to convert (about %s)", m.Plan.Est.Files, mb(m.Plan.Est.DDSBytes))
		}
		if m.Err != "" {
			line += "; " + m.Err
		}
		n.Add(line)
	}
	n.Truncate(render.DefaultLimit, "")
	if len(n.Children) == 0 {
		n.Line = "mods: none with textures"
	}
	if r.Est.Files > 0 {
		d.AddNext("archo textures optimize " + texArgs(a) + " --dry-run")
	}
	return d, nil
}

// texArgs repeats the command's mods and scope for the next step.
func texArgs(a *Args) string {
	var parts []string
	for _, p := range a.Pos {
		parts = append(parts, quoteArg(p))
	}
	if a.Bool("plan") {
		parts = append(parts, "--plan")
	}
	if a.Bool("all") {
		parts = append(parts, "--all")
	}
	if s := a.Str("scale"); s != "" {
		parts = append(parts, "--scale", s)
	}
	return strings.Join(parts, " ")
}

func texToolLine(root *render.Node, t *app.TextureTool) {
	if t == nil {
		return
	}
	if t.Err != "" {
		root.KV("encoder", t.Err)
		return
	}
	root.KV("encoder", fmt.Sprintf("todds %s (%s)", t.Version, t.Path))
}

func texOptimizeCmd(c *Ctx, a *Args) (*render.Doc, error) {
	sc, err := texScope(a)
	if err != nil {
		return nil, err
	}
	o, err := textureOptions(a)
	if err != nil {
		return nil, err
	}
	run := app.TextureRun{Refs: a.Pos, Scope: sc, Options: o, Force: a.Bool("force"), DryRun: a.Bool("dry-run")}
	if Progress != nil {
		run.Progress = func(p app.TextureProgress) {
			Progress(TextureProgressLine(p))
		}
	}
	r, err := c.App.OptimizeTextures(run)
	if err != nil {
		if errors.Is(err, app.ErrNoTodds) {
			return nil, fmt.Errorf("%w (%s)", err, textures.ToddsHome)
		}
		return nil, err
	}
	if r.DryRun {
		d, root := render.New(fmt.Sprintf("textures optimize: dry run, nothing written (%s)", app.TextureLabel(o)))
		texToolLine(root, r.Tool)
		root.KV("convert", TextureEstimateLine(r.Est))
		skipped := map[string]int{}
		for _, m := range r.Mods {
			if m.Plan != nil {
				for k, v := range m.Plan.Skipped {
					skipped[k] += v
				}
			}
		}
		if n := skipped[textures.SkipFresh]; n > 0 {
			root.KV("up to date", fmt.Sprintf("%d (--force converts them again)", n))
		}
		if n := skipped[textures.SkipOwnDDS] + skipped[textures.SkipOtherFmt]; n > 0 {
			root.KV("left alone", fmt.Sprintf("%d PNGs that share a name with a DDS of the mod's own or another format", n))
		}
		root.KV("writes", "a .dds next to each PNG, in the mods' folders; the PNGs stay; archo textures clean takes them back")
		if r.Est.Files > 0 {
			d.AddNext("archo textures optimize " + texArgs(a))
		}
		return d, nil
	}
	head := fmt.Sprintf("converted %d textures to DDS (%s written)", r.Converted, mb(r.Written))
	if r.Stopped {
		head = fmt.Sprintf("stopped: %d textures converted, %d not", r.Converted, r.Incomplete)
	}
	d, root := render.New(head)
	if r.Removed > 0 {
		root.KV("removed", fmt.Sprintf("%d DDS whose PNG is gone, or whose name failed in another folder", r.Removed))
	}
	if len(r.Failed) > 0 {
		n := root.KV("failed", fmt.Sprintf("%d (they keep their PNG)", len(r.Failed)))
		for _, f := range r.Failed {
			n.Addf("%s: %s: %s", f.Mod, f.Rel, f.Err)
		}
		n.Truncate(10, "")
	}
	return d, nil
}

func texCleanCmd(c *Ctx, a *Args) (*render.Doc, error) {
	sc, err := texScope(a)
	if err != nil {
		return nil, err
	}
	r, err := c.App.CleanTextures(a.Pos, sc, a.Bool("dry-run"))
	if err != nil {
		return nil, err
	}
	head := fmt.Sprintf("deleted %d DDS RimArcho wrote (%s); the game uses the PNGs again", r.Files, mb(r.Bytes))
	if r.DryRun {
		head = fmt.Sprintf("textures clean: dry run, would delete %d DDS (%s)", r.Files, mb(r.Bytes))
	}
	d, root := render.New(head)
	for _, m := range r.Mods {
		if m.Files > 0 {
			root.Addf("%s %s: %d (%s)", m.Ref, m.Name, m.Files, mb(m.Bytes))
		}
	}
	root.Truncate(render.DefaultLimit, "")
	if r.Left > 0 {
		root.KV("left", fmt.Sprintf("%d files changed since RimArcho wrote them: forgotten, not deleted", r.Left))
	}
	if r.DryRun && r.Files > 0 {
		d.AddNext("archo textures clean " + texArgs(a))
	}
	return d, nil
}

func texToolCmd(c *Ctx, a *Args) (*render.Doc, error) {
	if a.Bool("install") {
		o := c.App.OfferTodds()
		if Progress != nil {
			Progress(fmt.Sprintf("progress: downloading todds %s (%s) from %s", o.Version, mb(o.Bytes), o.URL))
		}
		exe, err := c.App.InstallTodds()
		if err != nil {
			return nil, err
		}
		d, root := render.New("installed todds " + textures.ToddsVersion)
		root.KV("program", exe)
		root.KV("licence", "MPL-2.0, "+textures.ToddsHome)
		return d, nil
	}
	t := c.App.TextureTool()
	d, root := render.New("the DDS encoder")
	texToolLine(root, t)
	if t.Err != "" {
		o := c.App.OfferTodds()
		root.KV("download", fmt.Sprintf("todds %s, %s, from %s into %s", o.Version, mb(o.Bytes), o.URL, o.Dir))
		d.AddNext("archo textures tool --install")
	}
	return d, nil
}
