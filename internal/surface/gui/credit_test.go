package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func guiCreditBatch(t *testing.T, e *testkit.Env) (int64, string) {
	t.Helper()
	root := filepath.Join(e.Root, "credit-mods")
	e.WriteMod(filepath.Join(root, "8100"), testkit.Sample("credit.mod8100", "Credit GUI"))
	if _, err := e.App.FolderAdd(source.SteamCMD, root); err != nil {
		t.Fatal(err)
	}
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	var instance string
	for _, m := range ix.Mods {
		if m.Kind == source.SteamCMD && m.Rel == "8100" {
			instance = m.ID
		}
	}
	if instance == "" {
		t.Fatal("fixture not scanned")
	}
	batch, err := e.App.Store.CreateCreditBatch(e.Now(), []store.CreditItem{{WorkshopID: "8100", InstanceID: instance, Name: "Credit GUI"}})
	if err != nil {
		t.Fatal(err)
	}
	en, _ := e.App.Env()
	for range 3 {
		launch := &store.LaunchRecord{EnvID: en.ID, Started: e.Now(), Method: "direct"}
		if err := e.App.Store.AddLaunchRecord(launch); err != nil {
			t.Fatal(err)
		}
		if err := e.App.Store.CountCreditSession(launch.ID, []string{instance}); err != nil {
			t.Fatal(err)
		}
	}
	return batch, instance
}

func TestCreditGUIContractAndDismiss(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	batch, _ := guiCreditBatch(t, e)
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake DLL")
	e.App.SteamWorker = func(string, []string) (string, error) { return "STATE 8100 5\nDONE state 1", nil }
	p, err := api.CreditPrompt()
	if err != nil || p == nil {
		t.Fatalf("prompt %+v %v", p, err)
	}
	b, _ := json.Marshal(p)
	want := fmt.Sprintf(`{"batchIDs":[%d],"modCount":1,"examples":["Credit GUI"]}`, batch)
	if string(b) != want {
		t.Fatalf("contract %s != %s", b, want)
	}
	r, err := api.CreditAnswer([]int64{batch}, false)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(r)
	if string(b) != fmt.Sprintf(`{"batchIDs":[%d],"state":"dismissed","taskID":""}`, batch) {
		t.Fatal(string(b))
	}
	if p, err := api.CreditPrompt(); err != nil || p != nil {
		t.Fatalf("dismissed %+v %v", p, err)
	}
}

func TestCreditGUITaskStopAndStartupResume(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	batch, _ := guiCreditBatch(t, e)
	// An unavailable client leaves a visible, cancellable task and durable obligation.
	r, err := api.CreditAnswer([]int64{batch}, true)
	if err != nil || r.TaskID == "" {
		t.Fatalf("answer %+v %v", r, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		items, _ := e.App.Store.CreditItems(batch)
		if items[0].Attempts > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not run")
		}
		time.Sleep(time.Millisecond)
	}
	var stop bool
	for _, row := range api.Tasks().Rows {
		if row.ID == r.TaskID {
			for _, act := range row.Acts {
				if act.ID == "stop" {
					stop = true
				}
			}
		}
	}
	if !stop {
		t.Fatal("no cancellation action")
	}
	if _, err := api.TaskAct("stop", []string{r.TaskID}, ""); err != nil {
		t.Fatal(err)
	}
	ended := endedTask(t, api, view.TaskCreditReplace)
	if ended.State != app.TaskStopped {
		t.Fatal(ended)
	}
	// Closing the window and reopening resumes from SQLite, even without tasks.json.
	api.creditWG.Wait()
	if err := os.Remove(filepath.Join(e.App.Home, "tasks.json")); err != nil {
		t.Fatal(err)
	}
	rapi := newAPI(e.App)
	defer rapi.Close()
	found := false
	for _, task := range rapi.tasks.List() {
		if task.Kind == view.TaskCreditReplace && !task.Ended() {
			found = true
		}
	}
	if !found {
		t.Fatal("startup lost the obligation")
	}
	bs, _ := e.App.Store.CreditBatches("replacing")
	if len(bs) != 1 {
		t.Fatal(bs)
	}
}

func TestSubscribeBulkGUIUsesBatch(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	exe := filepath.Join(e.Root, "tools", "steamcmd.exe")
	e.MkdirAll(filepath.Dir(exe))
	e.App.FindTool = func(string) (string, bool) { return exe, true }
	e.App.SteamCMDRun = func(_ string, args []string) (string, error) {
		e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", "8100"), testkit.Sample("credit.mod8100", "Credit GUI"))
		return "Success. Downloaded item 8100", nil
	}
	// Exercise JSON binding too: missing batch defaults to false for older callers.
	var req SubscriptionRequest
	if err := json.Unmarshal([]byte(`{"kind":"workshop","url":"8100","steamcmd":true,"batch":true}`), &req); err != nil {
		t.Fatal(err)
	}
	if _, err := api.Subscribe(req); err != nil {
		t.Fatal(err)
	}
	row := endedTask(t, api, view.TaskSubscribe)
	if row.State != app.TaskDone {
		t.Fatal(row)
	}
	bs, _ := e.App.Store.CreditBatches("counting")
	if len(bs) != 1 {
		t.Fatal(bs)
	}
	its, _ := e.App.Store.CreditItems(bs[0].ID)
	if len(its) != 1 || its[0].WorkshopID != "8100" || !strings.Contains(its[0].Name, "Credit") {
		t.Fatal(its)
	}
}

func TestCreditGUIProgressDuringSlowSteamCall(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	batch, _ := guiCreditBatch(t, e)
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake DLL")
	started, release := make(chan struct{}), make(chan struct{})
	e.App.SteamWorker = func(string, []string) (string, error) {
		close(started)
		<-release
		return "ITEM 8100 ok 1\nDONE subscribe 1", nil
	}
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	answer, err := api.CreditAnswer([]int64{batch}, true)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("Steam call did not start")
	}
	rows := api.Tasks().Rows
	found := false
	for _, row := range rows {
		if row.ID == answer.TaskID {
			found = true
			if len(row.Items) != 1 || row.Items[0].State != app.ItemRun || !row.Bar {
				t.Fatalf("progress invisible while Steam is slow %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("task missing")
	}
	if _, err := api.TaskAct("stop", []string{answer.TaskID}, ""); err != nil {
		t.Fatal(err)
	}
	close(release)
	released = true
	ended := endedTask(t, api, view.TaskCreditReplace)
	if ended.State != app.TaskStopped {
		t.Fatal(ended)
	}
	it, _ := e.App.Store.CreditItems(batch)
	if it[0].State != "subscribed" {
		t.Fatalf("stopping lost confirmed subscription %+v", it)
	}
}

func TestCreditQuestionRenderer(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for the credit question tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/credit.mjs").CombinedOutput(); err != nil {
		t.Fatalf("credit question: %v\n%s", err, out)
	}
}

func TestCreditCloseBoundsSlowWorkerAndKeepsCheckpoint(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	batch, _ := guiCreditBatch(t, e)
	e.WriteFile(filepath.Join(e.Game, "steam_api64.dll"), "fake DLL")
	started, release := make(chan struct{}), make(chan struct{})
	e.App.SteamWorker = func(string, []string) (string, error) {
		close(started)
		<-release
		return "ITEM 8100 ok 1\nDONE subscribe 1", nil
	}
	if _, err := api.CreditAnswer([]int64{batch}, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("worker not started")
	}
	closed := make(chan struct{})
	go func() { api.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(4 * time.Second):
		close(release)
		t.Fatal("window close waited for the Steam worker")
	}
	// The runner still owns its separate connection. Late confirmation is checkpointed,
	// while the closed GUI task remains stopped and no handoff can start.
	close(release)
	api.creditWG.Wait()
	items, err := e.App.Store.CreditItems(batch)
	if err != nil || items[0].State != "subscribed" {
		t.Fatalf("shutdown lost subscription confirmation: %+v %v", items, err)
	}
	for _, task := range api.tasks.List() {
		if task.Kind == view.TaskCreditReplace && task.State != app.TaskStopped {
			t.Fatalf("late completion changed closed task: %+v", task)
		}
	}
}

func TestCreditReplacementDiagnostics(t *testing.T) {
	lang := view.Lang()
	view.SetLang("zh")
	t.Cleanup(func() { view.SetLang(lang) })
	for _, tc := range []struct{ raw, want string }{
		{"retained: Workshop copy cannot be loaded in game B; shared lists unchanged", "游戏 game B 无法加载工坊副本"},
		{"retained: staging list list B is unreadable; repair it before replacing", "列表 list B 的暂存数据无法读取"},
		{"switched to Workshop; SteamCMD copy moved to recycle bin; texture records are bound to the old folder and content snapshot; optimise Workshop textures again", "请重新优化工坊副本的贴图"},
	} {
		if got := creditResultText(tc.raw); !strings.Contains(got, tc.want) {
			t.Fatalf("diagnostic %q: %q", tc.raw, got)
		}
	}
}
