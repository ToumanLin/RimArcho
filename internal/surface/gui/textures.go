package gui

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/textures"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 贴图优化 (08 §5.2, 06 §2.14): its page, and the runs of Mod 库存's 优化贴图. A run takes
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
	Running *TextureEvent `json:"running,omitempty"`
}

// Textures is the 贴图优化 page: scope "plan" or "all", scale a percent (100: full size).
func (s *API) Textures(scope string, scale int) (*TexturePage, error) {
	defer s.free.soon()
	ctx, cancel := s.beginTextureRead()
	defer cancel()
	s.texRead.mu.Lock()
	defer s.texRead.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope != app.TexAll {
		scope = app.TexPlan
	}
	r, targets, err := s.textureSession(scope)
	if err != nil {
		return nil, err
	}
	report, err := r.TextureSummaryTargets(ctx, targets, textures.Options{Scale: scale})
	if err != nil {
		return nil, err
	}
	out := &TexturePage{TexturePage: view.TexturePageReport(r, report, scope)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["textures"] {
		ev := s.texLast
		out.Running = &ev
	}
	return out, nil
}

// TexturesInstall downloads todds; the page asked the player first (source, size, folder).
func (s *API) TexturesInstall() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return settingQueued(s.queueSettingTaskLocked(view.TaskTodds, nil, "")), nil
}

// TexturesRun starts converting: the mods refs (Mod 库存's selection), else scope "plan" or
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
	return view.T("converting textures") + " · " + view.T("it goes on in the background"), nil
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
		out += view.Tf(" · %d failed (they keep their PNG)", failed)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy["textures"] {
		return "", errors.New(view.T("textures are being converted; wait for it to finish, or stop it"))
	}
	sc := app.TexPlan
	if scope == "all" {
		sc = app.TexAll
	}
	obj := view.T("the current plan")
	if scope == "all" {
		obj = view.T("every installed mod")
	}
	task := s.beginSelf(view.TaskTexClean, obj, nil)
	c, err := s.a.CleanTextures(nil, sc, false)
	s.lib, s.needs, s.board = nil, nil, nil
	if err != nil {
		s.endSelf(task, err, "")
		return "", err
	}
	s.endSelf(task, nil, view.Tf("deleted %d DDS RimArcho wrote (%s); the game uses the PNGs again", c.Files, view.SizeText(c.Bytes)))
	return view.Tf("deleted %d DDS RimArcho wrote (%s); the game uses the PNGs again", c.Files, view.SizeText(c.Bytes)), nil
}
