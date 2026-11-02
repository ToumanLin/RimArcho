package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"strings"
	"testing"
)

func TestUninstallConfirmation(t *testing.T) {
	t.Setenv("ARCHO_LANG", "")
	old := Lang()
	defer SetLang(old)
	for _, lang := range []string{"en", "zh"} {
		SetLang(lang)
		p := &app.UninstallPlan{EstimatedSizes: true, Steps: []*app.UninstallStep{
			{Key: app.UnPacks, Items: []*app.UninstallItem{
				{Do: app.DoTrash, Bytes: 1024}, {Do: app.DoTrash, Bytes: 2048},
				{Do: app.DoDelete, Bytes: 512}, {Do: app.DoKeep, Bytes: 4096},
				{Do: app.DoMove, Bytes: 8192, To: "synthetic-destination"},
			}},
			{Key: app.UnLinks, Items: []*app.UninstallItem{{Do: app.DoUnlink}}},
			{Key: app.UnData, Items: []*app.UninstallItem{{Do: app.DoLater, Bytes: 16384}}},
			{Key: app.UnTextures, Items: []*app.UninstallItem{{Do: app.DoDDS, Bytes: 256}}},
		}}
		pg := UninstallView(p, false)
		c := pg.Confirmation
		if c == nil || len(c.Changes) != 5 || len(c.Kept) != 2 {
			t.Fatalf("bad groups: %+v", c)
		}
		if row := c.Changes[0]; row.Count != 2 || row.Method != T("recycle bin") || !strings.HasPrefix(row.Size, "≈ ") {
			t.Fatalf("bad recycle summary: %+v", row)
		}
		if c.Changes[1].Method != T("delete directly") || c.Changes[3].Method != T("delete directly after closing") {
			t.Fatal("deletion methods lost")
		}
		if c.Changes[2].Size != T("size not measured") || c.Changes[4].Title == "" || c.Changes[4].Size == T("size not measured") {
			t.Fatal("metadata/DDS sizes or titles lost")
		}
		if c.Kept[1].Destinations[0] != "synthetic-destination" || c.Kept[1].Method != T("keep; move out") || c.Untouched == "" {
			t.Fatal("preserved content lost")
		}
		if UninstallView(p, true).Confirmation != nil {
			t.Fatal("execution report contains preview confirmation")
		}
		want := "uninstalling; you can close RimArcho when it finishes"
		if lang == "zh" {
			want = "正在卸载，完成后才能关闭"
		}
		if got := UninstallCloseReason(); got != want {
			t.Fatalf("%s reason = %q", lang, got)
		}
	}
}
