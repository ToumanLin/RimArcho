package gui

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/textures"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// Texture optimisation (08 §5.2, 06 §2.13): its page, and the runs of Mod library's optimise textures. A run takes
// minutes, so it is an `archo textures optimize` child, like a scan: the window keeps
// working, and "textures" events carry its progress (TextureEvent) and its end.

// TextureEvent is a run as it goes.
type TextureEvent struct {
	Text  string  `json:"text"`
	Frac  float64 `json:"frac"`
	Done  bool    `json:"done,omitempty"`
	Ok    bool    `json:"ok,omitempty"`
	Error string  `json:"error,omitempty"`
}

// TexturePage is the page with the run going on, if any.
type TexturePage struct {
	*view.TexturePage
	Request uint64        `json:"request"`
	Error   string        `json:"error,omitempty"`
	Running *TextureEvent `json:"running,omitempty"`
}

type textureMeasurement struct {
	mu      sync.Mutex
	request uint64
	page    *view.TexturePage
	err     string
}

// TexturesInfo is the page without reading a texture: the encoder and the scope's mods.
// The page opens with it and changing a choice asks only this; Textures reads the textures
// when the player asks (06 §2.13).
func (s *API) TexturesInfo(scope string, scale int) (*TexturePage, error) {
	if scope != app.TexAll {
		scope = app.TexPlan
	}
	a, err := s.detachedReader()
	if err != nil {
		return nil, err
	}
	defer a.Close()
	targets, err := a.TextureTargets(nil, scope)
	if err != nil {
		return nil, err
	}
	p := view.TexturePageInfo(a, scope, scale, a.TextureTool(), len(targets))
	out := &TexturePage{TexturePage: p}
	s.mu.Lock()
	if s.busy["textures"] {
		ev := s.texLast
		out.Running = &ev
	}
	s.mu.Unlock()
	return out, nil
}

// Textures returns choices immediately; disk reads run in the background (06 §3.1).
func (s *API) Textures(scope string, scale int) (*TexturePage, error) {
	if scope != app.TexAll {
		scope = app.TexPlan
	}
	m := &s.texMeasure
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := s.beginTextureRead()
	m.request++
	m.page, m.err = view.TexturePageLoading(scope, scale), ""
	request := m.request
	go func() {
		defer cancel()
		s.measureTextures(ctx, request, scope, scale)
	}()
	return s.textureSnapshot(), nil
}

// TextureMeasure polls only the current request; previous options have no result (06 §3.1).
func (s *API) TextureMeasure(request uint64) *TexturePage {
	s.texMeasure.mu.Lock()
	defer s.texMeasure.mu.Unlock()
	if request != s.texMeasure.request || s.texMeasure.page == nil {
		return nil
	}
	return s.textureSnapshot()
}

// textureSnapshot copies mutable rows with texMeasure.mu held.
func (s *API) textureSnapshot() *TexturePage {
	m := &s.texMeasure
	p := *m.page
	p.Rows = append([]view.TextureRow(nil), p.Rows...)
	out := &TexturePage{TexturePage: &p, Request: m.request, Error: m.err}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["textures"] {
		ev := s.texLast
		out.Running = &ev
	}
	return out
}

// publishTextureMeasure also guards the final write after a cancelled disk read (06 §3.1).
func (s *API) publishTextureMeasure(ctx context.Context, request uint64, update func(*textureMeasurement)) {
	m := &s.texMeasure
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() == nil && request == m.request {
		update(m)
	}
}

func (s *API) measureTextures(ctx context.Context, request uint64, scope string, scale int) {
	defer s.free.soon()
	s.texRead.mu.Lock()
	defer s.texRead.mu.Unlock()
	if ctx.Err() != nil {
		return
	}
	fail := func(err error) {
		s.publishTextureMeasure(ctx, request, func(m *textureMeasurement) {
			m.page.Loading, m.err = false, view.Humanize(nil, err.Error())
		})
	}
	r, targets, err := s.textureSession(scope)
	if err != nil {
		fail(err)
		return
	}
	rows := make([]view.TextureRow, 0, len(targets))
	index := make(map[string]int, len(targets))
	for _, target := range targets {
		index[target.Ref] = len(rows)
		rows = append(rows, view.TextureRow{Ref: target.Ref, Name: target.Mod.DisplayName(), Pending: true})
	}
	s.publishTextureMeasure(ctx, request, func(m *textureMeasurement) {
		m.page.Mods, m.page.Rows = len(targets), rows
	})
	report, err := r.TextureSummaryProgress(ctx, targets, textures.Options{Scale: scale}, func(mod *app.TextureMod) {
		row := view.TextureRowOf(mod)
		s.publishTextureMeasure(ctx, request, func(m *textureMeasurement) {
			m.page.Rows[index[mod.Ref]] = row
		})
	})
	if err != nil {
		fail(err)
		return
	}
	p := view.TexturePageReport(r, report, scope)
	s.publishTextureMeasure(ctx, request, func(m *textureMeasurement) { m.page = p })
}

// TexturesInstall downloads todds; the page asked the player first (source, size, folder).
func (s *API) TexturesInstall() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return settingQueued(s.queueSettingTaskLocked(view.TaskTodds, nil, "")), nil
}

// TexturesRun starts converting: the mods refs (Mod library's selection), else scope "plan" or
// "all"; scale a percent (100: full size). One run at a time.
func (s *API) TexturesRun(scope string, scale int, refs []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["textures"] {
		return "", errors.New(view.T("textures are being converted; wait for it to finish, or stop it"))
	}
	if g, _ := s.a.RunningGame(); g != nil {
		return "", errors.New(view.T("close the game first: it may be reading the textures"))
	}
	args := []string{"textures", "optimize"}
	switch {
	case len(refs) > 0:
		args = append(args, refs...)
	case scope == "all":
		args = append(args, "--all")
	default:
		args = append(args, "--plan")
	}
	if scale > 0 && scale < 100 {
		args = append(args, "--scale", strconv.Itoa(scale))
	}
	run := s.TexRunner
	if run == nil {
		home, env, exe := s.a.Home, s.a.EnvName, s.Exe
		run = func(args []string, progress func(string)) ([]string, error) {
			return app.ChildArcho(exe, home, env, args, progress)
		}
	}
	s.busy["textures"] = true
	s.texLast = TextureEvent{Text: view.T("converting textures")}
	obj := view.T("the current plan")
	switch {
	case len(refs) > 0:
		obj = view.Tf("%d mods", len(refs))
	case scope == "all":
		obj = view.T("every installed mod")
	}
	task := s.beginSelf(view.TaskTextures, obj, func() error { _, err := s.TexturesStop(); return err })
	go s.watchTextures(task, args, run)
	return view.T("converting textures") + "  " + view.T("it goes on in the background"), nil
}

var (
	texDoneRe    = regexp.MustCompile(`^converted (\d+) textures to DDS \((.+) written\)$`)
	texStoppedRe = regexp.MustCompile(`^stopped: (\d+) textures converted, (\d+) not$`)
	texFailedRe  = regexp.MustCompile(`^failed: (\d+) `)
)

func (s *API) watchTextures(task string, args []string, run func([]string, func(string)) ([]string, error)) {
	lines, err := run(args, func(l string) {
		p, ok := cmds.ParseTextureProgress(l)
		if !ok {
			return
		}
		ev := TextureEvent{Text: view.TextureProgressText(p)}
		if p.FilesTotal > 0 {
			ev.Frac = float64(p.Files) / float64(p.FilesTotal)
		}
		s.mu.Lock()
		s.texLast = ev
		s.mu.Unlock()
		s.tasks.Update(task, func(t *app.Task) { t.Frac, t.Phase = ev.Frac, ev.Text })
		s.send("textures", ev)
	})
	ev := TextureEvent{Done: true, Ok: err == nil, Frac: 1}
	if err != nil {
		ev.Error = view.Humanize(nil, err.Error())
	} else {
		ev.Text = textureDoneText(lines)
	}
	s.mu.Lock()
	s.busy["textures"], s.lib, s.needs, s.board = false, nil, nil, nil
	s.mu.Unlock()
	switch {
	case err != nil:
		s.endSelf(task, err, "")
	case s.tasks.StopAsked(task):
		s.tasks.Finish(task, app.TaskStopped, ev.Text)
	default:
		s.endSelf(task, nil, ev.Text)
	}
	s.send("textures", ev)
}

// textureDoneText is the run's last word in the interface's language.
func textureDoneText(lines []string) string {
	out := view.T("done")
	failed := 0
	for _, l := range lines {
		if m := texDoneRe.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[1])
			out = view.Tf("converted %d textures to DDS (%s written)", n, m[2])
		} else if m := texStoppedRe.FindStringSubmatch(l); m != nil {
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			out = view.Tf("stopped: %d textures converted, %d not", a, b)
		} else if m := texFailedRe.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			failed, _ = strconv.Atoi(m[1])
		}
	}
	if failed > 0 {
		out += view.Tf(", %d failed (they keep their PNG)", failed)
	}
	return out
}

// TexturesStop asks the run to stop between files: what it converted stays.
func (s *API) TexturesStop() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.busy["textures"] {
		return "", errors.New(view.T("nothing is being converted"))
	}
	if err := s.a.StopTextures(); err != nil {
		return "", err
	}
	return view.T("asked the conversion to stop"), nil
}

// TexturesClean deletes the DDS RimArcho wrote: scope "plan" or "all".
func (s *API) TexturesClean(scope string) (string, error) {
	return s.cleanTextures(scope, nil)
}

func (s *API) cleanTextures(scope string, refs []string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["textures"] {
		return "", errors.New(view.T("textures are being converted; wait for it to finish, or stop it"))
	}
	sc := app.TexPlan
	if scope == "all" {
		sc = app.TexAll
	}
	if len(refs) > 0 {
		sc = app.TexRefs
		refs = append([]string(nil), refs...)
	}
	obj := view.T("the current plan")
	if scope == "all" {
		obj = view.T("every installed mod")
	}
	if len(refs) > 0 {
		obj = view.Tf("%d mods", len(refs))
	}
	task := s.beginSelf(view.TaskTexClean, obj, nil)
	a, err := s.a.ReadSession()
	if err != nil {
		s.endSelf(task, err, "")
		return "", err
	}
	a.WorkspaceSnapshot()
	s.busy["textures"] = true
	s.texLast = TextureEvent{Text: view.TaskTitle(view.TaskTexClean)}
	go func() {
		defer a.Close()
		c, err := a.CleanTextures(refs, sc, false)
		s.mu.Lock()
		s.busy["textures"] = false
		s.mu.Unlock()
		s.invalidateInventory()
		text := ""
		if err == nil {
			text = view.Tf("deleted %d DDS RimArcho wrote (%s); the game uses the PNGs again", c.Files, view.SizeText(c.Bytes))
		}
		s.endSelf(task, err, text)
	}()
	return view.T("it goes on in the background"), nil
}
