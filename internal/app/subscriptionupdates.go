package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

// SourceCheck is cached locally: opening the page never contacts a source.
type SourceCheck struct {
	At     time.Time
	Result string
	Latest map[string]int64
}

func (a *App) SubscriptionSourceCheck(kind string) SourceCheck {
	var c SourceCheck
	_ = json.Unmarshal([]byte(a.Store.Meta(a.subscriptionCheckKey(kind))), &c)
	return c
}

func (a *App) subscriptionCheckKey(kind string) string {
	name := a.EnvName
	if env, err := a.Env(); err == nil {
		name = env.Name
	}
	return "subscriptions:check:" + name + ":" + kind
}

func (a *App) CheckSubscriptionSource(kind string) error {
	if kind != LibWorkshop && kind != LibSteamCMD && kind != LibGit && kind != LibPage && kind != LibLocal {
		return fmt.Errorf("unknown subscription source %q", kind)
	}
	c := a.SubscriptionSourceCheck(kind)
	c.At = a.Now()
	var failures []error
	if kind == LibLocal {
		_, err := a.Scan(false)
		failures = append(failures, err)
	} else if kind == LibWorkshop || kind == LibSteamCMD {
		ix, err := a.Index()
		if err != nil {
			return err
		}
		if c.Latest == nil {
			c.Latest = map[string]int64{}
		}
		for _, m := range ix.Mods {
			if m.Status != identity.Live || (kind == LibWorkshop && m.Kind != source.Workshop) || (kind == LibSteamCMD && m.Kind != source.SteamCMD) {
				continue
			}
			d, err := a.fetchWorkshop(m.WorkshopID())
			if err == nil && d.Result != 1 {
				err = fmt.Errorf("Workshop %s: result %d", m.WorkshopID(), d.Result)
			}
			if err != nil {
				failures = append(failures, err)
				continue
			}
			c.Latest[m.ID] = d.TimeUpdated
		}
	} else {
		ss, err := a.Subscriptions()
		if err != nil {
			return err
		}
		for _, s := range ss {
			if s.Record.Kind != kind {
				continue
			}
			op, err := a.CheckSubscriptions([]string{s.Ref})
			failures = append(failures, err)
			if err == nil && len(op.Items) > 0 && op.Items[0].Record.CheckError != "" {
				failures = append(failures, errors.New(op.Items[0].Record.CheckError))
			}
		}
	}
	err := errors.Join(failures...)
	c.Result = "check complete"
	if err != nil {
		c.Result = err.Error()
	}
	b, _ := json.Marshal(c)
	return errors.Join(err, a.Store.SetMeta(a.subscriptionCheckKey(kind), string(b)))
}

// UpdateWorkshopClient asks the running client to download; Steam owns completion.
func (a *App) UpdateWorkshopClient(ref string, dryRun bool) error {
	if running, err := a.GameRunning(); err != nil {
		return err
	} else if running != nil {
		return fmt.Errorf("the game is running (pid %d); close it before updating subscriptions", running.PID)
	}
	ix, err := a.Index()
	if err != nil {
		return err
	}
	m, err := ix.ResolveOne(ref)
	if err != nil {
		return err
	}
	if m.Kind != source.Workshop {
		return fmt.Errorf("%s is not a Workshop client mod", ref)
	}
	if dryRun {
		return nil
	}
	r, err := a.runSteam(steamworks.Download, []string{m.WorkshopID()})
	if err != nil {
		return err
	}
	if len(r.Items) != 1 || r.Items[0].Result != "ok" {
		return fmt.Errorf("Steam did not accept download of %s", strconv.Quote(m.DisplayName()))
	}
	return nil
}
