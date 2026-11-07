package view_test

import (
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// 16 §2.9: every copy gets RimArcho's first note, not only the locked versions it made.
func TestCopyNoteAutoEveryCopy(t *testing.T) {
	view.SetLang("zh")
	defer view.SetLang("en")
	day := time.Date(2026, 10, 19, 12, 0, 0, 0, time.Local)
	for _, c := range []struct {
		i    app.LibInstance
		want string
	}{
		{app.LibInstance{Source: app.LibWorkshop, Created: day}, "工坊订阅，10 月 19 日首次发现"},
		{app.LibInstance{Source: app.LibLocal, Created: day, UpstreamWorkshop: true, UpstreamUpdates: 2}, "放在游戏的 Mods 文件夹，10 月 19 日首次发现，之后工坊那份更新了 2 次"},
		{app.LibInstance{Source: app.LibLocal, Created: day, UpstreamWorkshop: true}, "放在游戏的 Mods 文件夹，10 月 19 日首次发现"},
		{app.LibInstance{Source: app.LibWorkshop, Created: day, States: []string{app.LibResidue}}, "已退订，Steam 留下了文件夹，10 月 19 日首次发现"},
		{app.LibInstance{Source: app.LibMonitor, Created: day}, "RimArcho 自带的伴生模组"},
		{app.LibInstance{Source: app.LibWorkshop, States: []string{app.LibMissing}}, ""},
		{app.LibInstance{Source: app.LibLocked, Created: day, Locked: &app.Locked{}, UpstreamUpdates: 3}, "该锁定版本创建于 10 月 19 日，之后出了 3 次更新"},
		{app.LibInstance{Source: app.LibLocked, Created: day, Locked: &app.Locked{Upstream: &store.Mod{}}}, "该锁定版本创建于 10 月 19 日，之后没有新的更新"},
	} {
		if got := view.CopyNoteAuto(&c.i); got != c.want {
			t.Errorf("%s %v: %q, want %q", c.i.Source, c.i.States, got, c.want)
		}
	}
}
