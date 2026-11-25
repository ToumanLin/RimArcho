package app_test

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestOversizedGameLog(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 300 MiB synthetic Player.log")
	}
	e := testkit.New(t)
	e.AddWorkshop("1001", testkit.Mod{Name: "Tick Spam", PackageID: "test.tickspam"})
	e.WriteModsConfig("1.6.4633", "ludeon.rimworld", "test.tickspam")
	e.AddEnv(false)
	e.MustRun("scan")

	const head = "RimWorld 1.6.4633 rev1261\n\n[Tick Spam] XML error at startup\n\n"
	const repeat = "[Tick Spam] InvalidOperationException: tick failure\n  at Mods.Tick.Fail () [0x00001] in <test>:0\n\n"
	const crash = "FatalException: game terminated\n  at Verse.Game.Stop () [0x00001] in <test>:0\n"
	path := filepath.Join(e.UserData, "Player.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 1<<20)
	write := func(text string) {
		t.Helper()
		if _, err := io.WriteString(w, text); err != nil {
			t.Fatal(err)
		}
	}
	write(head)
	chunk := strings.Repeat(repeat, 2048)
	const target = 300 << 20
	for n := 0; n < target/len(chunk); n++ {
		if n == target/len(chunk)/2 {
			write("MiddleException: this is outside the retained windows\n\n")
		}
		write(chunk)
	}
	write(crash)
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	rep, err := e.App.AnalyzeLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OmittedBytes < target-(9<<20) || rep.Trimmed || len(rep.Clusters) != 3 {
		t.Fatalf("oversized report: omitted %d, trimmed %t, clusters %d", rep.OmittedBytes, rep.Trimmed, len(rep.Clusters))
	}
	a, b, c := rep.Clusters[0], rep.Clusters[1], rep.Clusters[2]
	if !strings.Contains(a.First.Message, "at startup") || b.Count < 1000 || len(b.Lines) != 5 ||
		len(b.Attr) == 0 || b.Attr[0].Role != app.RoleSource || !strings.Contains(c.First.Message, "terminated") {
		t.Fatalf("unexpected clusters: %+v %+v %+v", a, b, c)
	}
	cur, err := app.ParseLogCursor(rep.Cursor)
	if err != nil || cur.Size < target-int64(len(chunk)) || cur.Size > target+1024 {
		t.Fatalf("cursor %s %+v: %v", rep.Cursor, cur, err)
	}
	if delta, err := e.App.Since(rep, cur); err != nil || delta.Stale != "" || len(delta.New) != 0 {
		t.Fatalf("unchanged sample: %+v, %v", delta, err)
	}
	more, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(more, "\nFatalException: after cursor\n"); err != nil {
		more.Close()
		t.Fatal(err)
	}
	if err := more.Close(); err != nil {
		t.Fatal(err)
	}
	fresh, err := e.App.AnalyzeLog(path)
	if err != nil {
		t.Fatal(err)
	}
	if delta, err := e.App.Since(fresh, cur); err != nil || delta.Stale == "" || len(delta.New) != len(fresh.Clusters) {
		t.Fatalf("growing sample: %+v, %v", delta, err)
	}

	live := e.App.FollowLog(path, "")
	if _, err := live.Poll(e.App); err != nil {
		t.Fatal(err)
	}
	if _, err := live.Close(e.App); err != nil {
		t.Fatal(err)
	}
	if live.OmittedBytes != fresh.OmittedBytes || len(live.Clusters()) != len(fresh.Clusters) {
		t.Fatalf("live read omitted %d, clusters %d", live.OmittedBytes, len(live.Clusters()))
	}

	p, err := e.App.PreviewLogCleanup(path, true)
	if err != nil || p.Size <= app.LogSizeLimit || p.Omitted != fresh.OmittedBytes || p.Remaining > 9<<20 {
		t.Fatalf("preview %+v: %v", p, err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 7, Started: e.Now().Add(-time.Minute)} }
	for _, trim := range []bool{true, false} {
		if err := e.App.ApplyLogCleanup(path, p.Token, trim); !errors.Is(err, app.ErrGameRunning) {
			t.Fatalf("cleanup while running, trim=%t: %v", trim, err)
		}
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	if err := e.App.ApplyLogCleanup(path, "old-token", true); !errors.Is(err, app.ErrLogCleanupChanged) {
		t.Fatalf("trim with stale confirmation: %v", err)
	}
	if err := e.App.ApplyLogCleanup(path, p.Token, false); !errors.Is(err, app.ErrLogCleanupChanged) {
		t.Fatalf("delete with a trim confirmation: %v", err)
	}
	if err := e.App.ApplyLogCleanup(path, p.Token, true); err != nil {
		t.Fatal(err)
	}
	trimmed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(trimmed)
	if int64(len(trimmed)) != p.Remaining || !strings.HasPrefix(body, head) ||
		!strings.Contains(body, crash) || strings.Count(body, "bytes omitted") != 1 ||
		strings.Count(body, "[Tick Spam] InvalidOperationException") != fresh.Clusters[1].Count {
		t.Fatal("trim did not preserve the head and tail with one marker")
	}
	report, err := e.App.AnalyzeLog(path)
	if err != nil || !report.Trimmed || report.OmittedBytes != p.Omitted {
		t.Fatalf("trimmed report: %+v, %v", report, err)
	}
	prev := filepath.Join(e.UserData, "Player-prev.log")
	e.WriteFile(prev, head)
	if err := os.Truncate(prev, app.LogSizeLimit+1); err != nil {
		t.Fatal(err)
	}
	preview := e.MustRun("log", "--delete", "--prev", "--dry-run")
	if !strings.Contains(preview, "permanently delete") {
		t.Fatal(preview)
	}
	del, err := e.App.PreviewLogCleanup(prev, false)
	if err != nil {
		t.Fatal(err)
	}
	e.MustRun("log", "--delete", "--prev", "--yes", "--token", del.Token)
	if _, err := os.Stat(prev); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted previous log: %v", err)
	}
}
