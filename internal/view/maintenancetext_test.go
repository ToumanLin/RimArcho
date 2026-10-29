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
		"database compacted: freed 7.0 MB (now 5.0 MB)":                                 "数据库已压缩：释放了 7.0 MB，现在 5.0 MB",
		"cannot compact database: it needs 1.2 GB of free disk space":                   "无法压缩数据库：需要 1.2 GB 可用磁盘空间",
		"cannot compact database: game main is running (pid 123); close the game first": "无法压缩数据库：游戏 main 正在运行（进程 123），请先关闭游戏",
		"database is busy; try compacting again later":                                  "数据库正忙，请稍后再压缩",
	} {
		if got := view.Text(from); got != want {
			t.Fatalf("%q: %q", from, got)
		}
	}
}
