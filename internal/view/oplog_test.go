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
	// One mark, only where something went wrong: the launch's own, the refusal's; none on
	// the recycled Mod.
	if launch.Mark != oplog.Failed || old.Mark != oplog.Refused || trash.Mark != "" {
		t.Fatalf("marks %q %q %q", launch.Mark, old.Mark, trash.Mark)
	}
}

// An action that went well but has a failed step shows the step's mark in its one mark
// column; a quick delete says nowhere to get it back (16 §2.16).
func TestOperationsMarkOfAStep(t *testing.T) {
	e := testkit.New(t)
	h := e.App.Ops.Begin(oplog.Row{Op: oplog.Uninstall, Subject: "RimArcho"})
	h.Child(oplog.Row{Op: oplog.Trash, Subject: "Saves", Result: oplog.Refused})
	h.End(oplog.OK, "")
	s := e.App.Ops.Begin(oplog.Row{Op: oplog.SubscriptionRemove, Subject: "Fat Bodies"})
	s.Child(oplog.Row{Op: oplog.Delete, Subject: "3771446780"})
	s.End(oplog.OK, oplog.Permanent)
	p, err := view.Operations(e.App)
	if err != nil || len(p.Rows) != 2 {
		t.Fatal(p, err)
	}
	sub, un := p.Rows[0], p.Rows[1]
	if un.Mark != oplog.Refused || !un.Bad || sub.Mark != "" || sub.Back != "" || sub.Steps[0].Back != "" || sub.Icon != "bin" {
		t.Fatalf("uninstall %+v, subscription %+v", un, sub)
	}
}

// A save batch's subject is a count stored in English ("1 save"); the list says it in the
// player's language (VM 2026-11-23: "1 save" in the Chinese window).
func TestOperationsSaveCountInLanguage(t *testing.T) {
	e := testkit.New(t)
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	e.App.Ops.Add(oplog.Row{Op: oplog.SaveTrash, Subject: "1 save"})
	e.App.Ops.Add(oplog.Row{Op: oplog.SaveBackup, Subject: "3 saves"})
	e.App.Ops.Add(oplog.Row{Op: oplog.Trash, Subject: "1 save"})
	p, err := view.Operations(e.App)
	if err != nil {
		t.Fatal(err)
	}
	saves := string([]rune{0x4e2a, 0x5b58, 0x6863}) // "saves" counted in Chinese
	got := []string{p.Rows[0].Subject, p.Rows[1].Subject, p.Rows[2].Subject}
	if got[0] != "1 save" || got[1] != "3 "+saves || got[2] != "1 "+saves {
		t.Fatalf("subjects %q", got)
	}
}
