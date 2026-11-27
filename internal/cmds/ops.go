package cmds

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// The operation log (13 §1.3): what RimArcho did to the player's files, newest first, from
// every surface. MCP list_operations prints the same text.
func init() {
	register(&Command{Name: "ops", Summary: "what RimArcho did to your files (the operation log), newest first", MaxArgs: 0, Run: opsCmd,
		Flags: []Flag{
			{Name: "since", Value: "<when>", Help: "only rows since <n>h, <n>d or YYYY-MM-DD"},
			{Name: "op", Value: "<code>", Help: "only one operation (trash, delete, subscription.remove, workshop.unsubscribe, modsconfig.write, link.create, link.remove, save.rename, save.copy, save.backup, save.trash, update.apply, update.rollback, uninstall, launch, plan.sync, looks.choice, looks.build, looks.style, looks.stylebuild) or a family (save, link, update, plan, looks)"},
			{Name: "failed", Help: "only failed, refused and partly done rows"},
			{Name: "all", Help: "every row kept (default: the newest 20)"},
		}})
}

// OpsDefault is how many rows archo ops prints without --all.
const OpsDefault = 20

func opsCmd(c *Ctx, a *Args) (*render.Doc, error) {
	q := oplog.Query{Op: strings.TrimSpace(a.Str("op")), Failed: a.Bool("failed")}
	if q.Op != "" && !knownOp(q.Op) {
		return nil, &UsageError{Msg: fmt.Sprintf("unknown --op %q: use %s, or a family: save, link, update", q.Op, strings.Join(oplog.Codes, ", "))}
	}
	if s := a.Str("since"); s != "" {
		t, err := opsSince(c, s)
		if err != nil {
			return nil, err
		}
		q.Since = t
	}
	if !a.Bool("all") {
		q.Limit = OpsDefault + 1
	}
	rows, err := c.App.Operations(q)
	if err != nil {
		return nil, err
	}
	more := false
	if q.Limit > 0 && len(rows) > OpsDefault {
		rows, more = rows[:OpsDefault], true
	}
	d, root := OpsDoc(c, rows)
	if more {
		root.Add("… older rows: archo ops --all")
	}
	return d, nil
}

// OpsDoc is the rows as archo ops prints them.
func OpsDoc(c *Ctx, rows []oplog.Row) (*render.Doc, *render.Node) {
	failed := 0
	for _, r := range rows {
		if oplog.Failure(r.Result) {
			failed++
		}
	}
	head := "operations: " + render.Plural(len(rows), "row") + ", newest first"
	if failed > 0 {
		head += fmt.Sprintf(" (%d failed or refused)", failed)
	}
	d, root := render.New(head)
	if len(rows) == 0 {
		root.Add("nothing recorded yet: RimArcho records what it does to your files from now on")
		return d, root
	}
	for _, r := range rows {
		h, more := oplog.Text(r, c.App.Loc)
		n := root.Add(h)
		for _, m := range more {
			n.Add(m)
		}
	}
	return d, root
}

func knownOp(op string) bool {
	op = strings.TrimSuffix(op, ".")
	if slices.Contains(oplog.Codes, op) {
		return true
	}
	for _, c := range oplog.Codes {
		if strings.HasPrefix(c, op+".") {
			return true
		}
	}
	return false
}

// opsSince reads <n>h, <n>d or YYYY-MM-DD.
func opsSince(c *Ctx, s string) (time.Time, error) {
	for unit, d := range map[string]time.Duration{"h": time.Hour, "d": 24 * time.Hour} {
		if v, ok := strings.CutSuffix(s, unit); ok {
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return time.Time{}, &UsageError{Msg: "--since takes <n>h, <n>d or YYYY-MM-DD"}
			}
			return c.App.Now().Add(-time.Duration(n) * d), nil
		}
	}
	t, err := time.ParseInLocation("2006-01-02", s, c.App.Loc)
	if err != nil {
		return time.Time{}, &UsageError{Msg: "--since takes <n>h, <n>d or YYYY-MM-DD"}
	}
	return t, nil
}
