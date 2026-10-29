package view_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/view"
)

func TestMaintenanceText(t *testing.T) {
	old := view.Lang()
	view.SetLang("zh")
	defer view.SetLang(old)
	for from, want := range map[string]string{
		"database compacted: 12.0 MB → 5.0 MB; freed 7.0 MB":                            "数据库已压缩：12.0 MB → 5.0 MB，释放了 7.0 MB",
		"cannot compact database: game main is running (pid 123); close the game first": "无法压缩数据库：游戏 main 正在运行（进程 123），请先关闭游戏",
		"database is busy; try compacting again later":                                  "数据库正忙，请稍后再压缩",
	} {
		if got := view.Text(from); got != want {
			t.Fatalf("%q: %q", from, got)
		}
	}
}
