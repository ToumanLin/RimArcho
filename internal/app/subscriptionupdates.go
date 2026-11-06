package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/subscribe"
)

// SourceCheck is cached locally: opening the page never contacts a source.
type SourceCheck struct {
	At       time.Time
	Result   string
	Failures []SourceFailure
	Latest   map[string]int64
}

func (a *App) SubscriptionSourceCheck(kind string) SourceCheck {
	var c SourceCheck
	value := a.Store.Meta(a.subscriptionCheckKey(kind))
	if value == "" && (kind == LibSteamCMD || kind == LibGit || kind == LibPage) {
		if e, err := a.Env(); err == nil {
			value = a.Store.Meta("subscriptions:check:" + e.Name + ":" + kind)
		}
	}
	_ = json.Unmarshal([]byte(value), &c)
	return c
}

func (a *App) subscriptionCheckKey(kind string) string {
	// Subscription folders are shared by every game (03 §2), so automatic
	// results must survive switching games too. Workshop-client state stays scoped.
	if kind == LibSteamCMD || kind == LibGit || kind == LibPage {
		return "subscriptions:check:shared:" + kind
	}
	name := a.EnvName
	if env, err := a.Env(); err == nil {
		name = env.Name
	}
	return "subscriptions:check:" + name + ":" + kind
}

type SourceFailure struct {
	ID, Name, Reason string
	Code             int
}

// Gone: Steam answered that the item is removed from the Workshop (the author deleted it or made it private).
func (f SourceFailure) Gone() bool { return f.Code == 9 }

// CheckFailures counts the items that could not be checked, leaving out the gone ones.
func CheckFailures(fs []SourceFailure) int {
	n := 0
	for _, f := range fs {
		if !f.Gone() {
			n++
		}
	}
	return n
}

func (a *App) CheckSubscriptionSource(kind string, pageReader ...func(string) (*subscribe.PageInfo, error)) error {
	if kind != LibWorkshop && kind != LibSteamCMD && kind != LibGit && kind != LibPage && kind != LibLocal {
		return fmt.Errorf("unknown subscription source %q", kind)
	}
	c := a.SubscriptionSourceCheck(kind)
	c.At = a.Now()
	c.Failures = nil
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
		var want []*store.Mod
		var ids []string
		for _, m := range ix.Mods {
			if m.Status != identity.Live || (kind == LibWorkshop && m.Kind != source.Workshop) || (kind == LibSteamCMD && m.Kind != source.SteamCMD) {
				continue
			}
			want = append(want, m)
			ids = append(ids, m.WorkshopID())
		}
		answers := a.workshopDetails(a.subscriptionContext(), ids)
		for _, m := range want {
			ans := answers[m.WorkshopID()]
			switch {
			case ans.err != nil:
				c.Failures = append(c.Failures, SourceFailure{ID: m.WorkshopID(), Name: m.DisplayName(), Reason: ans.err.Error()})
			case ans.d.Result != 1:
				c.Failures = append(c.Failures, SourceFailure{ID: m.WorkshopID(), Name: m.DisplayName(), Reason: WorkshopResultText(ans.d.Result), Code: ans.d.Result})
				if ans.d.Result == 9 {
					delete(c.Latest, m.ID) // nothing left to update to
				}
			default:
				c.Latest[m.ID] = ans.d.TimeUpdated
			}
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
			op, err := a.CheckSubscriptions([]string{s.Ref}, pageReader...)
			failures = append(failures, err)
			if err == nil && len(op.Items) > 0 && op.Items[0].Record.CheckError != "" {
				failures = append(failures, errors.New(op.Items[0].Record.CheckError))
			}
		}
	}
	// Items gone from the Workshop were checked: Steam said so. They are listed, not failures.
	broken := CheckFailures(c.Failures)
	if broken > 0 {
		failures = append(failures, fmt.Errorf("%d Workshop items could not be checked", broken))
	}
	err := errors.Join(failures...)
	c.Result = "check complete"
	if err != nil {
		c.Result = err.Error()
		if broken > 0 {
			c.Result = "some checks failed"
		}
	}
	b, _ := json.Marshal(c)
	return errors.Join(err, a.Store.SetMeta(a.subscriptionCheckKey(kind), string(b)))
}

// UpdateWorkshopClient asks the running client to download; Steam owns completion.
func (a *App) UpdateWorkshopClient(ref string, dryRun bool) error {
	return a.UpdateWorkshopClients([]string{ref}, dryRun)
}

// UpdateWorkshopClients asks the running client to download every copy in one Steam call
// (update all, 16 §2.9); Steam owns completion. The error names the items Steam refused.
func (a *App) UpdateWorkshopClients(refs []string, dryRun bool) error {
	if err := a.refuseSubscriptionGameRunning(); err != nil {
		return err
	}
	ix, err := a.Index()
	if err != nil {
		return err
	}
	var ids []string
	names := map[uint64]string{}
	for _, ref := range refs {
		m, err := ix.ResolveOne(ref)
		if err != nil {
			return err
		}
		if m.Kind != source.Workshop {
			return fmt.Errorf("%s is not a Workshop client mod", ref)
		}
		id, err := strconv.ParseUint(m.WorkshopID(), 10, 64)
		if err != nil {
			return fmt.Errorf("%s has no Workshop ID", ref)
		}
		if _, seen := names[id]; !seen {
			ids = append(ids, m.WorkshopID())
		}
		names[id] = m.DisplayName()
	}
	if dryRun || len(ids) == 0 {
		return nil
	}
	r, err := a.runSteam(steamworks.Download, ids)
	if err != nil {
		return err
	}
	ok := map[uint64]bool{}
	for _, it := range r.Items {
		ok[it.ID] = ok[it.ID] || it.Result == "ok"
	}
	var refused []string
	for _, id := range ids {
		n, _ := strconv.ParseUint(id, 10, 64)
		if !ok[n] {
			refused = append(refused, strconv.Quote(names[n]))
		}
	}
	if len(refused) > 0 {
		return fmt.Errorf("Steam did not accept download of %s", strings.Join(refused, ", "))
	}
	return nil
}
