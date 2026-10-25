package gui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestNeedsLinkHealthRefreshAndConfirmedSnapshot(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	target := filepath.Join(e.Root, "temporary-mod")
	e.MkdirAll(target)
	link := filepath.Join(e.Game, "Mods", "_Archo_unproven")
	e.Junction(link, target)
	_ = api.needsLocked()
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	needs := api.needsLocked()
	var action view.Action
	for _, item := range needs.Items {
		for _, act := range item.Actions {
			if act.Kind == view.ActRepairLinks {
				action = act
			}
		}
	}
	if action.Kind == "" || len(action.Lines) == 0 || action.Confirm == "" || !strings.Contains(action.Warn, "temporary-mod") {
		t.Fatal("missing confirmation evidence")
	}
	if !source.IsLink(link) {
		t.Fatal("needs page removed link")
	}
	// Repeated needs reads preserve this preview, but execution rechecks the entry.
	for i := 0; i < 35; i++ {
		_ = api.needsLocked()
	}
	e.MkdirAll(target)
	if _, err := api.DoAction(action, 0); err == nil {
		t.Fatal("stale preview executed after target restored")
	}
	if !source.IsLink(link) {
		t.Fatal("healthy link removed")
	}
}

func TestNeedsLinkRowsAndRepairPreview(t *testing.T) {
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang("en") })
	e, api, _ := fixture(t)
	root, err := source.RealPath(e.Root)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "missing-target")
	path := filepath.Join(e.Game, "Mods", "_Archo_env_old-id")
	e.Junction(path, target)
	var item *view.Need
	needs := api.needsLocked()
	for i := range needs.Items {
		if len(needs.Items[i].LinkGroups) > 0 {
			item = &needs.Items[i]
			break
		}
	}
	if item == nil {
		t.Fatal("no grouped link panel")
	}
	group := item.LinkGroups[0]
	if group.Game == "" || group.Path == "" || len(group.Rows) != 1 {
		t.Fatalf("game header: %+v", group)
	}
	row := group.Rows[0]
	if row.Name != filepath.Base(path) || filepath.Base(row.Path) != filepath.Base(path) || row.Target != target || row.Reason != "目标不存在" || row.Action != "保留" {
		t.Fatalf("row: %+v", row)
	}
	if len(item.Actions) != 1 || len(item.Actions[0].LinkGroups) != 1 {
		t.Fatal("no structured repair preview")
	}
	preview := item.Actions[0].LinkGroups[0].Rows[0]
	if preview.Name != row.Name || preview.Reason != row.Reason || preview.Action != "将删除" {
		t.Fatalf("preview row: %+v", preview)
	}
	if strings.Contains(item.Why, "prefix only") || strings.Contains(item.Actions[0].Warn, "ancestor is accessible") {
		t.Fatal("technical evidence leaked into row copy")
	}
	for _, tc := range []struct {
		code  string
		owned bool
		want  string
	}{
		{app.CodeLinkForeign, false, "指向未知位置，未确认是 RimArcho 建的"},
		{app.CodeLinkUnknown, false, "无法读取（离线或无权限）"},
		{app.CodeModsDuplicate, true, "重复入口"},
	} {
		if got := view.LinkReason(&app.GameLink{Code: tc.code, Owned: tc.owned}); got != tc.want {
			t.Fatalf("reason: %q", got)
		}
	}
}
