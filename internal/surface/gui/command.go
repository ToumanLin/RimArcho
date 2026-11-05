package gui

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmdline"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

const guiHistoryKey = "ui.gui.command.history"

type CommandResult struct {
	Sequence uint64 `json:"sequence"`
	Line     string `json:"line"`
	Running  bool   `json:"running"`
	Output   string `json:"output"`
	Code     int    `json:"code"`
	// Uninstall: `uninstall --yes` started the protected run (16 §2.14); the page takes over.
	Uninstall bool `json:"uninstall,omitempty"`
}
type commandWorker struct {
	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
	cancel context.CancelFunc
	result CommandResult
}

func (s *API) closeCommand() {
	w := &s.command
	w.mu.Lock()
	w.closed = true
	if w.cancel != nil {
		w.cancel()
	}
	w.mu.Unlock()
	w.wg.Wait()
}
func (s *API) CommandStatus() CommandResult {
	w := &s.command
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.result
}

// RunCLI acknowledges immediately. The worker owns its connection for the entire
// invocation; page reads never wait on its execution or output formatting.
func (s *API) RunCLI(line string) (CommandResult, error) {
	argv := cmdline.Argv(line)
	env, argv, bad := cmdline.Env(argv)
	if bad != "" {
		return CommandResult{}, errors.New(bad)
	}
	if len(argv) == 0 {
		return CommandResult{}, errors.New(view.T("not a command here"))
	}
	if err := cmds.Check(argv); err != nil {
		return CommandResult{}, err
	}
	s.uninstall.mu.Lock()
	uninstalling := s.uninstall.status.Running
	s.uninstall.mu.Unlock()
	if uninstalling {
		return CommandResult{}, errors.New(view.T("uninstalling; you can close RimArcho when it finishes"))
	}
	// `uninstall --yes` goes through the same guarded run as the page; without it, a preview.
	if argv[0] == "uninstall" {
		options, run, err := cmds.ParseUninstallRequest(argv)
		if err != nil {
			return CommandResult{}, err
		}
		if run {
			if err := s.startUninstall(options); err != nil {
				return CommandResult{}, err
			}
			return CommandResult{Line: line, Uninstall: true}, nil
		}
	}
	if argv[0] == "log" && (slices.Contains(argv, "--follow") || slices.ContainsFunc(argv, func(x string) bool { return strings.HasPrefix(x, "--follow=") })) {
		return CommandResult{}, errors.New(view.T("run log --follow in a terminal"))
	}
	w := &s.command
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return CommandResult{}, errors.New(view.T("the window is closing"))
	}
	if w.result.Running {
		return CommandResult{}, errors.New(view.T("the last command is still running"))
	}
	a, err := s.detachedReader()
	if err != nil {
		return CommandResult{}, err
	}
	if env != "" {
		if env != a.EnvName {
			a.OpenPlan, a.PlanName = 0, ""
		}
		a.EnvName = env
	}
	child := (argv[0] == "scan" || argv[0] == "analyze") && !slices.Contains(argv, "--help") && !slices.Contains(argv, "-h")
	if child {
		s.mu.Lock()
		if s.busy[argv[0]] || s.needsClosed {
			s.mu.Unlock()
			a.Close()
			return CommandResult{}, errors.New(view.T("the last command is still running"))
		}
		s.busy[argv[0]] = true
		s.mu.Unlock()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.result = CommandResult{Sequence: w.result.Sequence + 1, Line: line, Running: true}
	initial := w.result
	// Only a command still running after a second gets a row in Background tasks (where it can be
	// stopped); quick ones answer in the panel and leave no record.
	var taskMu sync.Mutex
	task, finished := "", false
	slow := time.AfterFunc(time.Second, func() {
		taskMu.Lock()
		defer taskMu.Unlock()
		if !finished {
			task = s.beginSelf("command", line, func() error { cancel(); return nil })
		}
	})
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer a.Close()
		defer cancel()
		var out string
		var code int
		if child {
			s.filesMu.Lock()
			if argv[0] == "scan" && s.ScanRunner != nil {
				out, err = s.ScanRunner(a.EnvName)
				if err != nil {
					out += "error: " + err.Error() + "\n"
					code = cmds.ExitError
				}
			} else {
				childArgs := slices.Clone(argv)
				if plan := a.OpenPlanName(); argv[0] == "analyze" && plan != "" {
					childArgs = append(childArgs, "--open-plan", plan)
				}
				out, code = app.ChildCommandContext(ctx, s.Exe, a.Home, a.EnvName, childArgs)
			}
			s.filesMu.Unlock()
		} else {
			s.filesMu.RLock()
			out, code = cmds.ExecuteContext(ctx, a, argv)
			s.filesMu.RUnlock()
		}
		if code != cmds.ExitUsage {
			hist := cmdline.Remember(cmdline.DecodeHistory(a.Store.Meta(guiHistoryKey)), strings.TrimSpace(line))
			if b, e := json.Marshal(hist); e == nil {
				_ = a.Store.SetMeta(guiHistoryKey, string(b))
			}
		}
		s.mu.Lock()
		if child {
			s.busy[argv[0]] = false
		}
		s.board, s.needs, s.lib = nil, nil, nil
		if code == cmds.ExitOK && len(argv) >= 2 && (argv[0] == "env" || argv[0] == "plan") && argv[1] == "use" {
			s.a.EnvName, s.a.PlanName, s.a.OpenPlan = "", "", 0
		}
		if code == cmds.ExitOK && len(argv) >= 3 && argv[0] == "config" && argv[1] == "set" && argv[2] == "ui.lang" {
			view.SetLang(cmds.Setting(a, "ui.lang"))
		}
		if a.Uninstalled {
			s.a.Uninstalled = true
		}
		s.mu.Unlock()
		s.subRevision.Add(1)
		slow.Stop()
		taskMu.Lock()
		finished = true
		if task != "" {
			if code == 0 {
				s.endSelf(task, nil, "")
			} else {
				s.endSelf(task, errors.New(out), "")
			}
		}
		taskMu.Unlock()
		w.mu.Lock()
		w.result.Output, w.result.Code, w.result.Running = out, code, false
		w.cancel = nil
		w.mu.Unlock()
	}()
	return initial, nil
}

type CommandCompletion struct {
	Candidates []cmdline.Candidate `json:"candidates"`
	From       int                 `json:"from"`
	To         int                 `json:"to"`
}

func (s *API) CommandComplete(line string, cursor int, current string, selected []string) (CommandCompletion, error) {
	a, err := s.detachedReader()
	if err != nil {
		return CommandCompletion{}, err
	}
	defer a.Close()
	c := cmdline.Context{App: a}
	if st, e := a.Loadout(); e == nil {
		for _, m := range st.Ix.Mods { // stable index order, same references as the CLI
			x := cmdline.Mod{ID: m.ID, Ref: st.Ix.Ref(m.ID), Name: m.DisplayName(), Package: m.PackageID}
			c.Mods = append(c.Mods, x)
			if x.ID == current {
				v := x
				c.Current = &v
			}
			if slices.Contains(selected, x.ID) {
				c.Selected = append(c.Selected, x)
			}
		}
	}
	cs, from, to := cmdline.Complete(c, line, cursor)
	return CommandCompletion{cs, from, to}, nil
}

type CommandHistoryResult struct {
	At    int    `json:"at"`
	Text  string `json:"text"`
	Draft string `json:"draft"`
}

func (s *API) CommandHistory(at, delta int, text, draft string) (CommandHistoryResult, error) {
	a, e := s.detachedReader()
	if e != nil {
		return CommandHistoryResult{}, e
	}
	defer a.Close()
	h := cmdline.DecodeHistory(a.Store.Meta(guiHistoryKey))
	if at < 0 {
		at = len(h)
	}
	at = min(at, len(h))
	at, text, draft = cmdline.Step(h, at, delta, text, draft)
	return CommandHistoryResult{at, text, draft}, nil
}
