package cmds

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rimarcho/rimarcho/internal/app"
	"github.com/rimarcho/rimarcho/internal/render"
	"github.com/rimarcho/rimarcho/internal/store"
)

const pathSep = os.PathSeparator

func init() {
	register(&Command{Name: "identity list", Summary: "open identity questions (\"is this #xxxx back?\")", MaxArgs: 0,
		Flags: []Flag{{Name: "all", Help: "include resolved questions"}}, Run: identityList})
	register(&Command{Name: "identity resolve", Args: "<q:n> same|copy|unrelated", Summary: "answer an identity question", MinArgs: 2, MaxArgs: 2, Run: identityResolve})
	register(&Command{Name: "lineage split", Args: "<ref>", Summary: "move an instance into a lineage of its own", MinArgs: 1, MaxArgs: 1, Run: lineageSplit})
	register(&Command{Name: "lineage merge", Args: "<ref> <ref>", Summary: "merge the second lineage into the first", MinArgs: 2, MaxArgs: 2, Run: lineageMerge})
	register(&Command{Name: "gc", Summary: "remove expired tombstones and superseded snapshots", MaxArgs: 0,
		Flags: []Flag{{Name: "dry-run", Help: "only report"}}, Run: gcCmd})
}

func unix(s int64) time.Time {
	if s == 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}

// parseSince accepts scan:<n>, <n>d and YYYY-MM-DD.
func parseSince(c *Ctx, s string, f *store.EventFilter) error {
	switch {
	case strings.HasPrefix(s, "scan:"):
		n, err := strconv.ParseInt(s[5:], 10, 64)
		if err != nil {
			return &UsageError{Msg: "--since scan:<n> needs a number"}
		}
		f.MinScan = n
	case strings.HasSuffix(s, "d"):
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || n < 0 {
			return &UsageError{Msg: "--since <n>d needs a number of days"}
		}
		f.SinceTime = c.App.Now().Add(-time.Duration(n) * 24 * time.Hour)
	default:
		t, err := time.ParseInLocation("2006-01-02", s, c.App.Loc)
		if err != nil {
			return &UsageError{Msg: "--since takes scan:<n>, <n>d or YYYY-MM-DD"}
		}
		f.SinceTime = t
	}
	return nil
}

func identityList(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	qs, err := c.App.Store.Questions(ix.Env.ID, a.Bool("all"))
	if err != nil {
		return nil, err
	}
	open := 0
	for _, q := range qs {
		if q.Status == "open" {
			open++
		}
	}
	head := fmt.Sprintf("questions: %d open", open)
	if a.Bool("all") {
		head += fmt.Sprintf(", %d resolved", len(qs)-open)
	}
	d, root := render.New(head)
	for _, q := range qs {
		line := fmt.Sprintf("q:%d  score %.2f  scan:%d", q.ID, q.Score, q.ScanID)
		if q.Status != "open" {
			line += "  [resolved: " + q.Resolution + "]"
		}
		n := root.Add(line)
		var rows [][]string
		for _, x := range []struct{ role, id string }{{"new:", q.InstanceID}, {"was it:", q.CandidateID}} {
			if m := ix.ByID[x.id]; m != nil {
				rows = append(rows, append([]string{x.role}, modRow(c, ix, m)...))
			} else {
				rows = append(rows, []string{x.role, ix.Ref(x.id), "(gone)"})
			}
		}
		n.AddColumns(rows)
	}
	root.Truncate(render.DefaultLimit/2, "archo identity list --all")
	for _, q := range qs {
		if q.Status == "open" {
			d.AddNext(fmt.Sprintf("archo identity resolve q:%d same|copy|unrelated", q.ID))
			break
		}
	}
	return d, nil
}

func identityResolve(c *Ctx, a *Args) (*render.Doc, error) {
	qid, err := app.ParseQuestionRef(a.Pos[0])
	if err != nil {
		return nil, err
	}
	before, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	q, err := c.App.ResolveQuestion(qid, strings.ToLower(a.Pos[1]))
	if err != nil {
		return nil, err
	}
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("q:%d  resolved: %s", q.ID, q.Resolution))
	cand := ix.ByID[q.CandidateID]
	switch q.Resolution {
	case "same":
		root.Addf("%s merged into %s; it keeps its ID, lineage and history", before.Ref(q.InstanceID), ix.Ref(q.CandidateID))
		if cand != nil {
			addMods(c, root, ix, []*store.Mod{cand}, 0, "")
			d.AddNext("archo mod " + ix.Ref(cand.ID))
		}
	case "copy":
		root.Addf("%s joined %s as a confirmed copy", ix.Ref(q.InstanceID), ix.LRef(cand.LineageID))
		addMods(c, root, ix, ix.ByLineage[cand.LineageID], 0, "")
	default:
		root.Addf("%s and %s stay unrelated", ix.Ref(q.InstanceID), ix.Ref(q.CandidateID))
	}
	return d, nil
}

func lineageSplit(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	m, err := ix.ResolveOne(a.Pos[0])
	if err != nil {
		return nil, err
	}
	oldLin := ix.LRef(m.LineageID)
	id, err := c.App.SplitLineage(ix, m)
	if err != nil {
		return nil, err
	}
	ix, err = c.App.Index()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%s %s  split", ix.Ref(m.ID), m.DisplayName()))
	root.KV("lineage", fmt.Sprintf("%s (was %s)", ix.LRef(id), oldLin))
	d.AddNext("archo mod " + ix.Ref(m.ID))
	return d, nil
}

func lineageMerge(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	into, err := ix.ResolveLineage(a.Pos[0])
	if err != nil {
		return nil, err
	}
	from, err := ix.ResolveLineage(a.Pos[1])
	if err != nil {
		return nil, err
	}
	fromRef := ix.LRef(from)
	if err := c.App.MergeLineages(ix, into, from); err != nil {
		return nil, err
	}
	ix, err = c.App.Index()
	if err != nil {
		return nil, err
	}
	d, root := render.New(fmt.Sprintf("%s  merged %s into it", ix.LRef(into), fromRef))
	addMods(c, root.Section("members"), ix, ix.ByLineage[into], 0, "")
	return d, nil
}

func gcCmd(c *Ctx, a *Args) (*render.Doc, error) {
	ix, err := c.App.Index()
	if err != nil {
		return nil, err
	}
	dry := a.Bool("dry-run")
	r, cutoff, err := c.App.GC(dry)
	if err != nil {
		return nil, err
	}
	verb := "removed"
	if dry {
		verb = "would remove"
	}
	d, root := render.New(fmt.Sprintf("gc: %s %s (tombstones older than %d days, before %s)", verb,
		render.Plural(len(r.Instances), "tombstone"), c.App.Config.TombstoneDays, c.App.FormatDate(cutoff)))
	var gone []*store.Mod
	for _, id := range r.Instances {
		if m := ix.ByID[id]; m != nil {
			gone = append(gone, m)
		}
	}
	addMods(c, root, ix, gone, render.DefaultLimit, "")
	if r.Kept > 0 {
		root.KV("kept", fmt.Sprintf("%d expired tombstones referenced by open questions (archo identity list)", r.Kept))
	}
	root.KV("snapshots", fmt.Sprintf("%d superseded (%d file rows)", r.Snapshots, r.Files))
	return d, nil
}
