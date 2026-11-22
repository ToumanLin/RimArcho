package view_test

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/oplog"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// What RimArcho did (16 §2.16): steps sit inside their action, a failed step makes the
// action stand out, the failure count is of actions, and ok rows say where to get things back.
func TestOperationsPage(t *testing.T) {
	e := testkit.New(t)
	ops := e.App.Ops
	if p, err := view.Operations(e.App); err != nil || p.Count != 0 || p.Empty == "" {
		t.Fatalf("empty page %+v, %v", p, err)
	}
	ops.Add(oplog.Row{Op: oplog.Trash, Subject: "Old Mod", Target: `C:\Mods\Old Mod`})
	h := ops.Begin(oplog.Row{Op: oplog.Launch, Subject: "default", Origin: oplog.Agent})
	h.Child(oplog.Row{Op: oplog.ModsConfigWrite, Subject: "default", Detail: "v7, 3 mods, launch"})
	h.Child(oplog.Row{Op: oplog.LinkCreate, Subject: "B", Result: oplog.Failed, Detail: "access denied"})
	h.End(oplog.Failed, "access denied")
	e.Advance(-48 * time.Hour)
	ops.Add(oplog.Row{Op: oplog.SaveRename, Subject: "Colony", Result: oplog.Refused})
	e.Advance(48 * time.Hour)

	p, err := view.Operations(e.App)
	if err != nil {
		t.Fatal(err)
	}
	if p.Count != 3 || p.Failed != 2 || len(p.Rows[1].Steps) != 2 {
		t.Fatalf("count %d, failed %d, rows %+v", p.Count, p.Failed, p.Rows)
	}
	launch, trash, old := p.Rows[1], p.Rows[2], p.Rows[0]
	if !launch.Bad || launch.Icon != "play" || launch.Origin != oplog.Agent || launch.Steps[1].Back != "in the list's history, as v7" && launch.Steps[0].Back != "in the list's history, as v7" {
		t.Fatalf("launch %+v", launch)
	}
	if trash.Bad || trash.Back != "in the system recycle bin" || trash.Icon != "bin" || trash.Day != "today" {
		t.Fatalf("trash %+v", trash)
	}
	if old.Op != oplog.SaveRename || !old.Bad || old.Day == "today" {
		t.Fatalf("old %+v", old)
	}
	if len(p.Origins) == 0 || len(p.Results) == 0 {
		t.Fatalf("facets %+v %+v", p.Results, p.Origins)
	}
}
