package gui

import (
	"context"
	"errors"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/accounts"
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func settingQueued(t app.Task) string {
	return view.Tf("%s, %s: in the %s queue", view.TaskTitle(t.Kind), t.Obj, "RimArcho")
}

// queueSettingTaskLocked captures the target at submission. Slow work uses its own
// connection and never holds the GUI lock (16 §3.1 rule 9, 16 §2.16).
func (s *API) queueSettingTaskLocked(kind string, argv []string, env string, resume ...app.Task) app.Task {
	snapshot := &app.App{Store: s.a.Store}
	snapshot.ReadOptions(s.a)
	if e, err := s.a.Env(); err == nil {
		snapshot.EnvName = e.Name
	}
	if env != "" {
		snapshot.EnvName = env
	}
	if kind == view.TaskSourceDecompile && snapshot.OpenPlan == 0 && snapshot.PlanName == "" {
		if e, err := snapshot.Env(); err == nil {
			snapshot.OpenPlan = snapshot.Store.CurrentPlanID(e.ID)
		}
	}
	obj := "RimArcho"
	if len(argv) == 3 && argv[0] == "deps" {
		obj = argv[2]
	} else if kind == view.TaskCache && len(argv) > 2 {
		obj = argv[2]
	} else if kind == view.TaskGC {
		obj = snapshot.EnvName
	} else if (kind == view.TaskSubscribe || kind == view.TaskSubscriptionFrom) && len(argv) > 2 {
		obj = argv[2]
	}
	request := app.Task{Kind: kind, Obj: obj, Args: argv, Env: snapshot.EnvName}
	if len(resume) > 0 {
		request = resume[0]
	}
	return s.tasks.EnqueueSelf(request, func(id string) (string, error) {
		r, err := snapshot.ReadSession()
		if err != nil {
			return "", err
		}
		defer r.Close()
		s.tasks.Update(id, func(t *app.Task) { t.Phase = view.TaskPending(kind) })
		text, err := s.runSettingTask(r, id, kind, argv)
		s.mu.Lock()
		s.board, s.needs, s.lib = nil, nil, nil
		if kind == view.TaskData {
			s.a.DropCommunityData()
		}
		if kind == view.TaskCache {
			s.a.DropLogCaches()
		}
		s.mu.Unlock()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return "", err
			}
			return "", errors.New(view.Humanize(nil, err.Error()))
		}
		return text, nil
	})
}

func (s *API) runSettingTask(a *app.App, id, kind string, argv []string) (string, error) {
	progress := func(done, total int64) {
		s.tasks.Update(id, func(t *app.Task) {
			t.Phase = view.SizeText(done)
			if total > 0 {
				t.Phase = view.Tf("%s of %s", view.SizeText(done), view.SizeText(total))
				t.Frac = min(0.99, float64(done)/float64(total))
			}
		})
	}
	switch kind {
	case view.TaskSourceDecompile, view.TaskSourceClean:
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.tasks.SetStop(id, func() error { cancel(); return nil })
		if kind == view.TaskSourceClean {
			p, err := a.CleanSourceLibrary(ctx, true)
			if err != nil {
				return "", err
			}
			return view.Tf("removed %d folders (%s)", p.Folders, view.SizeText(p.Bytes)), nil
		}
		err := a.DecompileActive(ctx, func(done, total int, file string) {
			s.tasks.Update(id, func(t *app.Task) {
				t.Done, t.Total, t.Phase = done, total, file
				if total > 0 {
					t.Frac = float64(done) / float64(total)
				}
			})
		})
		return view.T("the current list's mods are in the source library"), err
	case view.TaskVacuum:
		defer s.free.soon()
		if len(argv) != 2 {
			return executeSettingTask(a, argv)
		}
		before, after, err := a.DBVacuum()
		if err != nil {
			return "", err
		}
		return view.Tf("database compacted: freed %s (now %s)", view.SizeText(max(0, before-after)), view.SizeText(after)), nil
	case view.TaskSourceCheck:
		if argv[1] == app.LibPage && a.Store.Meta("account:loverslab:enabled") == "true" {
			release, err := s.beginAccount()
			if err != nil {
				return "", err
			}
			defer release()
			unlock := accounts.Lock(a.AccountsDir())
			defer unlock()
			ll, err := s.loversLab(a)
			if err != nil {
				return "", err
			}
			return view.SourceCheckResult(a, argv[1], func(page string) (*subscribe.PageInfo, error) {
				if subscribe.LoversLabPage(page) {
					return ll.PageInfo(context.Background(), page)
				}
				return subscribe.ReadPage(context.Background(), a.HTTP, page)
			})
		}
		return view.SourceCheckResult(a, argv[1])
	case view.TaskWorkshopUpdate:
		return view.T("Steam will download this Workshop update"), a.UpdateWorkshopClient(argv[1], false)
	case view.TaskSubscribe, view.TaskSubscriptionFrom:
		return s.runSubscriptionTask(a, id, kind, argv)
	case view.TaskSelfCheck:
		return s.runSelfCheck(a)
	case view.TaskSelfUpdate:
		return s.runSelfUpdate(a, id)
	case view.TaskSteamCMD, view.TaskTodds:
		var path string
		var err error
		if len(argv) == 3 && argv[0] == "deps" && argv[1] == "install" {
			path, err = a.InstallDep(argv[2], progress)
		} else if kind == view.TaskSteamCMD {
			path, err = a.InstallSteamCMDProgress(progress)
		} else {
			path, err = a.InstallToddsProgress(progress)
		}
		return view.Tf("downloaded to %s", path), err
	case view.TaskData:
		// Keep optional datasets and flags under the command parser's validation.
		if len(argv) != 2 {
			return executeSettingTask(a, argv)
		}
		res, err := a.UpdateDataProgress(nil, false, func(key string, done int) {
			s.tasks.Update(id, func(t *app.Task) { t.Total, t.Done, t.Phase = len(app.Datasets), done, key })
		})
		var parts []string
		for _, u := range res {
			if u.Err != nil {
				parts = append(parts, u.Dataset.Key+": "+u.Err.Error())
			} else {
				parts = append(parts, view.Tf("%s: %d entries", u.Dataset.Key, u.Info.Entries))
			}
		}
		if err != nil {
			return "", errors.New(err.Error() + "  " + strings.Join(parts, "  "))
		}
		return view.T("community data updated") + "  " + strings.Join(parts, "  "), nil
	default:
		return executeSettingTask(a, argv)
	}
}

func executeSettingTask(a *app.App, argv []string) (string, error) {
	text, code := cmds.Execute(a, argv)
	text = strings.TrimSpace(text)
	if code != cmds.ExitOK {
		return "", errors.New(text)
	}
	line, _, _ := strings.Cut(text, "\n")
	return line, nil
}
