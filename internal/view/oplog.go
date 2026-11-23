package view

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/oplog"
)

// "What RimArcho did" (16 §2.16, 13 §1.3): the operation log as the window and the terminal
// show it. A multi-step action's own rows (a launch's ModsConfig.xml write and links) sit
// inside it as steps. Codes stay English in the data; this model says them in the player's
// language. It never offers an undo: Back only says where an item can be got back.

// OpsPage is the list.
type OpsPage struct {
	Rows   []OpRow `json:"rows"` // newest first
	Count  int     `json:"count"`
	Failed int     `json:"failed"` // actions that failed, were refused or partly done, or have such a step
	Kept   string  `json:"kept"`   // how long rows are kept (the heading's hover)
	Empty  string  `json:"empty,omitempty"`
}

// OpRow is one action.
type OpRow struct {
	ID         int64  `json:"id"`
	Op         string `json:"op"`
	Icon       string `json:"icon"` // the window's icon key (icons.js)
	What       string `json:"what"`
	Subject    string `json:"subject"`
	Result     string `json:"result"` // ok, failed, refused, partial, started
	ResultText string `json:"resultText"`
	Bad        bool   `json:"bad,omitempty"` // failed, refused or partial: it stands out
	// Mark is the result the row's one mark shows: its own when it did not succeed, else
	// that of its first failed step; "" for a row that went well (no mark).
	Mark       string  `json:"mark,omitempty"`
	Origin     string  `json:"origin"`
	OriginText string  `json:"originText"`
	Day        string  `json:"day"`  // today, yesterday, or the date
	Time       string  `json:"time"` // HH:MM
	At         int64   `json:"at"`   // Unix milliseconds
	Target     string  `json:"target,omitempty"`
	Detail     string  `json:"detail,omitempty"`
	Back       string  `json:"back,omitempty"` // where the item can be got back
	Steps      []OpRow `json:"steps,omitempty"`
	Search     string  `json:"search"`
}

// OpsLimit is how many rows the list reads; the log keeps at most oplog.MaxRows.
const OpsLimit = oplog.MaxRows

var opWhat = map[string]string{
	oplog.Trash:              "moved to the recycle bin",
	oplog.Delete:             "deleted for good",
	oplog.SubscriptionRemove: "removed a subscription",
	oplog.Unsubscribe:        "unsubscribed on Steam",
	oplog.ModsConfigWrite:    "wrote the list to the game",
	oplog.LinkCreate:         "made links",
	oplog.LinkRemove:         "removed links",
	oplog.SaveRename:         "renamed a save",
	oplog.SaveCopy:           "copied a save",
	oplog.SaveBackup:         "backed up saves",
	oplog.SaveTrash:          "recycled saves",
	oplog.UpdateApply:        "updated RimArcho",
	oplog.UpdateRollback:     "put RimArcho back",
	oplog.Uninstall:          "uninstalled",
	oplog.Launch:             "started the game",
}

var opIcon = map[string]string{
	oplog.Trash: "bin", oplog.Delete: "bin", oplog.SubscriptionRemove: "bin", oplog.Unsubscribe: "workshop", oplog.ModsConfigWrite: "plan", oplog.LinkCreate: "link", oplog.LinkRemove: "unlink",
	oplog.SaveRename: "saves", oplog.SaveCopy: "saves", oplog.SaveBackup: "saves", oplog.SaveTrash: "bin",
	oplog.UpdateApply: "rescan", oplog.UpdateRollback: "rescan", oplog.Uninstall: "close", oplog.Launch: "play",
}

var opResult = map[string]string{
	oplog.OK: "done", oplog.Failed: "failed", oplog.Refused: "refused", oplog.Partial: "partly done", oplog.Started: "did not finish",
}

var opOrigin = map[string]string{
	oplog.GUI: "window", oplog.TUI: "terminal", oplog.CLI: "command line", oplog.MCP: "an agent (MCP)", oplog.Agent: "Archo", oplog.Auto: "automatic",
}

// OpWhat is an op code in the player's words.
func OpWhat(op string) string {
	if w, ok := opWhat[op]; ok {
		return T(w)
	}
	return op
}

// Operations reads the list: the newest OpsLimit rows, steps inside their action.
func Operations(a *app.App) (*OpsPage, error) {
	rows, err := a.Operations(oplog.Query{Limit: OpsLimit})
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	loc := a.Loc
	if loc == nil {
		loc = time.Local
	}
	p := &OpsPage{Rows: []OpRow{}, Kept: Tf("kept %d days, at most %d rows", int(oplog.MaxAge/(24*time.Hour)), oplog.MaxRows)}
	parents := map[int64]bool{}
	for _, r := range rows {
		if r.Batch == 0 {
			parents[r.ID] = true
		}
	}
	steps := map[int64][]OpRow{}
	for _, r := range rows {
		o := opRow(r, now, loc)
		if r.Batch != 0 && parents[r.Batch] {
			steps[r.Batch] = append(steps[r.Batch], o)
			continue
		}
		p.Rows = append(p.Rows, o)
	}
	for i := range p.Rows {
		r := &p.Rows[i]
		r.Steps = steps[r.ID]
		// A failed step makes its action stand out, whatever the action's own result.
		for _, s := range r.Steps {
			r.Search += " " + s.Search
			if s.Bad && !r.Bad {
				r.Bad = true
			}
			if s.Bad && r.Mark == "" {
				r.Mark = s.Result
			}
		}
		if r.Bad {
			p.Failed++
		}
	}
	p.Count = len(p.Rows)
	if p.Count == 0 {
		p.Empty = T("RimArcho has not changed any of your files yet")
	}
	return p, nil
}

func opRow(r oplog.Row, now time.Time, loc *time.Location) OpRow {
	at := r.At.In(loc)
	o := OpRow{ID: r.ID, Op: r.Op, Icon: opIcon[r.Op], What: OpWhat(r.Op), Subject: opSubject(r), Result: r.Result,
		ResultText: T(opResult[r.Result]), Bad: oplog.Failure(r.Result), Origin: r.Origin, OriginText: T(opOrigin[r.Origin]),
		Time: at.Format("15:04"), At: r.At.UnixMilli(), Target: r.Target, Detail: r.Detail, Back: opBack(r)}
	if o.Icon == "" {
		o.Icon = "log"
	}
	if r.Result != oplog.OK {
		o.Mark = r.Result
	}
	if o.ResultText == "" {
		o.ResultText = r.Result
	}
	if o.OriginText == "" {
		o.OriginText = r.Origin
	}
	today := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	switch {
	case !at.Before(today):
		o.Day = T("today")
	case !at.Before(today.AddDate(0, 0, -1)):
		o.Day = T("yesterday")
	default:
		o.Day = at.Format("2006-01-02")
	}
	o.Search = strings.ToLower(strings.Join([]string{o.What, r.Op, r.Subject, r.Target, r.Detail, o.ResultText, o.OriginText}, " "))
	return o
}

// opSubject is the row's subject in the player's language: save batches store their count
// as "1 save" / "N saves" (app.plural), which the list must not show in English.
func opSubject(r oplog.Row) string {
	if r.Op == oplog.SaveTrash || r.Op == oplog.SaveBackup {
		if r.Subject == "1 save" {
			return T("1 save")
		}
		if n, ok := strings.CutSuffix(r.Subject, " saves"); ok {
			if k, err := strconv.Atoi(n); err == nil {
				return Tf("%d saves", k)
			}
		}
	}
	return r.Subject
}

// opBack says, in the player's words, where an ok row's item can be got back (oplog.Where).
func opBack(r oplog.Row) string {
	if oplog.Where(r) == "" {
		return ""
	}
	switch r.Op {
	case oplog.Trash, oplog.SaveTrash, oplog.SubscriptionRemove:
		return T("in the system recycle bin")
	case oplog.ModsConfigWrite:
		if v := oplog.VersionOf(r.Detail); v != "" {
			return Tf("in the list's history, as %s", v)
		}
		return T("in the list's history")
	case oplog.UpdateApply:
		return T("the previous release is on GitHub")
	}
	return ""
}

// OpsLines is the newest n rows in the CLI's text (archo ops), for the feedback attachment
// (16 §2.14): a maintainer reads the same text an agent would.
func OpsLines(a *app.App, n int) []string {
	rows, err := a.Operations(oplog.Query{Limit: n})
	if err != nil {
		return []string{err.Error()}
	}
	loc := a.Loc
	if loc == nil {
		loc = time.Local
	}
	var out []string
	for _, r := range rows {
		head, more := oplog.Text(r, loc)
		out = append(out, head)
		for _, m := range more {
			out = append(out, "  "+m)
		}
	}
	if total := a.Ops.Count(); total > len(rows) {
		out = append(out, fmt.Sprintf("… %d older rows (archo ops --all)", total-len(rows)))
	}
	return out
}
