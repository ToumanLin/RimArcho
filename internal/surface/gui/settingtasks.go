package gui

import (
	"errors"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func settingQueued(t app.Task) string {
	return view.Tf("%s · %s: in the %s queue", view.TaskTitle(t.Kind), t.Obj, "RimArcho")
}

// queueSettingTaskLocked captures the target at submission. Slow work uses its own
// connection and never holds the GUI lock (06 §3.1 rule 9, 06 §2.16).
func (s *API) queueSettingTaskLocked(kind string, argv []string, env string) app.Task {
	snapshot := &app.App{Store: s.a.Store}
	snapshot.ReadOptions(s.a)
	if e, err := s.a.Env(); err == nil {
		snapshot.EnvName = e.Name
	}
	if env != "" {
		snapshot.EnvName = env
	}
	obj := "RimArcho"
	if kind == view.TaskCache && len(argv) > 2 {
		obj = argv[2]
	} else if kind == view.TaskGC {
		obj = snapshot.EnvName
	}
	request := app.Task{Kind: kind, Obj: obj, Args: argv, Env: snapshot.EnvName}
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
	case view.TaskSteamCMD, view.TaskTodds:
		var path string
		var err error
		if kind == view.TaskSteamCMD {
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
			return "", errors.New(err.Error() + " · " + strings.Join(parts, " · "))
		}
		return view.T("community data updated") + " · " + strings.Join(parts, " · "), nil
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

func init() {
	view.Register(map[string]string{
		"downloaded to %s":       "已下载到 %s",
		"%s: %d entries":         "%s：%d 条",
		"community data updated": "社区数据已更新",
	})
}
