package upgradewin

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func TestNilSafety(t *testing.T) {
	var w *Window
	w.Update("x", 0.5)
	w.Close()
	w.Close()
}

func TestSmoke(t *testing.T) {
	if os.Getenv("RIMARCHO_UPGRADEWIN_SMOKE") != "1" {
		t.Skip("set RIMARCHO_UPGRADEWIN_SMOKE=1 to show the native window")
	}
	if runtime.GOOS != "windows" {
		if Open("RimArcho", "Upgrading database") != nil {
			t.Fatal("Open must return nil outside Windows")
		}
		t.Skip("native window requires Windows")
	}
	w := Open("RimArcho", "正在升级数据库…")
	if w == nil {
		t.Fatal("native window could not be created")
	}
	t.Cleanup(w.Close)
	for i := 0; i <= 4; i++ {
		w.Update("正在升级数据库…", float64(i)/4)
		time.Sleep(200 * time.Millisecond)
	}
	w.Close()
	w.Close()
	w.Update("Finished", 1)
}
