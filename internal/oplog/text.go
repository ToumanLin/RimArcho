package oplog

import (
	"fmt"
	"strings"
	"time"
)

// Text is a row as `archo ops`, list_operations and the feedback attachment print it
// (13 §1.3): a head line, then the target, the detail and where to get the item back, each
// indented on its own line when there is one.
func Text(r Row, loc *time.Location) (string, []string) {
	if loc == nil {
		loc = time.Local
	}
	head := fmt.Sprintf("#%d %s %-5s %-20s %-7s %s", r.ID, r.At.In(loc).Format("2006-01-02 15:04:05"), r.Origin, r.Op, r.Result, r.Subject)
	head = strings.TrimRight(head, " ")
	if r.Ms > 0 {
		head += "  " + Dur(r.Ms)
	}
	if r.Batch != 0 {
		head += fmt.Sprintf("  (part of #%d)", r.Batch)
	}
	var more []string
	if r.Target != "" && r.Target != r.Subject {
		more = append(more, "at: "+r.Target)
	}
	if r.Detail != "" {
		more = append(more, "detail: "+r.Detail)
	}
	if w := Where(r); w != "" {
		more = append(more, "back: "+w)
	}
	return strings.TrimRight(head, " "), more
}

// Where says where the item an ok row touched can be got back from, in English (the CLI's
// words; view translates them): the recycle bin for trash, the plan's history for a
// ModsConfig.xml write. "" when there is no such place. Never an undo: the log only points.
func Where(r Row) string {
	if r.Result != OK && r.Result != Partial {
		return ""
	}
	switch r.Op {
	case Trash, SaveTrash:
		return "the recycle bin"
	case SubscriptionRemove:
		if !strings.HasPrefix(r.Detail, Permanent) {
			return "the recycle bin"
		}
	case ModsConfigWrite:
		if v := VersionOf(r.Detail); v != "" {
			return "the plan's history (archo history; archo loadout checkout " + v + " lists it again)"
		}
		return "the plan's history (archo history)"
	case UpdateApply:
		return "the previous release on GitHub"
	case PlanSync:
		return "the plan's history (archo undo while it is the current plan)"
	}
	return ""
}

// VersionOf is the "v<n>" a modsconfig.write detail begins with, or "".
func VersionOf(detail string) string {
	f, _, _ := strings.Cut(detail, ",")
	if len(f) > 1 && f[0] == 'v' && strings.Trim(f[1:], "0123456789") == "" {
		return f
	}
	return ""
}

// Dur is a duration in milliseconds as the lists show it: "850 ms", "12.4 s", "3 min 5 s".
// The units are the same in every language.
func Dur(ms int64) string {
	switch {
	case ms < 1000:
		return fmt.Sprintf("%d ms", ms)
	case ms < 60_000:
		return strings.Replace(fmt.Sprintf("%.1f s", float64(ms)/1000), ".0 s", " s", 1)
	}
	m, s := ms/60_000, (ms%60_000)/1000
	if s == 0 {
		return fmt.Sprintf("%d min", m)
	}
	return fmt.Sprintf("%d min %d s", m, s)
}
