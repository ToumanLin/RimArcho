package app_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/deps"
	"github.com/ToumanLin/RimArcho/internal/loadout"
	"github.com/ToumanLin/RimArcho/internal/source"
	"github.com/ToumanLin/RimArcho/internal/steamworks"
	"github.com/ToumanLin/RimArcho/internal/store"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// All downloads and account calls are fakes; even the DLL is a fixture text file.
func creditEnv(t *testing.T, ids ...string) (*testkit.Env, int64, []store.CreditItem) {
	t.Helper()
	e := setup(t, false)
	exe := filepath.Join(e.Root, "tools", deps.SteamCMDExecutable())
	e.MkdirAll(filepath.Dir(exe))
	e.App.FindTool = func(string) (string, bool) { return exe, true }
	e.App.SteamCMDRun = func(_ string, args []string) (string, error) {
		var out []string
		for _, id := range workshopScriptIDs(t, exe, args) {
			if id == "9999" {
				out = append(out, "ERROR! Download item 9999 failed (Failure).")
				continue
			}
			e.WriteMod(filepath.Join(args[1], "steamapps", "workshop", "content", "294100", id), testkit.Sample("credit.mod"+id, "Credit "+id))
			out = append(out, "Success. Downloaded item "+id)
		}
		return strings.Join(out, "\n"), nil
	}
	op, err := e.App.SubscribeWorkshopBatch(ids, true, true, false)
	if op == nil {
		t.Fatal(err)
	}
	bs, err := e.App.Store.CreditBatches("")
	if err != nil || len(bs) != 1 {
		t.Fatalf("batches %+v %v", bs, err)
	}
	items, err := e.App.Store.CreditItems(bs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return e, bs[0].ID, items
}

func creditLaunch(t *testing.T, e *testkit.Env, ids ...string) int64 {
	t.Helper()
	en, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	r := &store.LaunchRecord{EnvID: en.ID, Started: e.Now(), Method: "direct"}
	if err := e.App.Store.AddLaunchRecord(r); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.CountCreditSession(r.ID, ids); err != nil {
		t.Fatal(err)
	}
	return r.ID
}
func creditDue(t *testing.T, e *testkit.Env, items []store.CreditItem) {
	t.Helper()
	var ids []string
	for _, it := range items {
		ids = append(ids, it.InstanceID)
	}
	for range 3 {
		creditLaunch(t, e, ids...)
	}
}
func creditStart(t *testing.T, e *testkit.Env, batch int64, items []store.CreditItem) {
	t.Helper()
	creditDue(t, e, items)
	if err := e.App.CreditAnswer([]int64{batch}, true); err != nil {
		t.Fatal(err)
	}
}
func creditClient(t *testing.T, e *testkit.Env, items []store.CreditItem) {
	t.Helper()
	steamDLL(e)
	for _, it := range items {
		e.AddWorkshop(it.WorkshopID, testkit.Sample("credit.mod"+it.WorkshopID, "Credit "+it.WorkshopID))
	}
	var calls [][]string
	e.App.SteamWorker = fakeSteamWorker(&calls)
}
func creditRecycle(e *testkit.Env, path string) error {
	dir := filepath.Join(e.Root, "recycled")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(dir, filepath.Base(path)))
}
func creditItem(t *testing.T, e *testkit.Env, batch int64) store.CreditItem {
	t.Helper()
	items, err := e.App.Store.CreditItems(batch)
	if err != nil || len(items) == 0 {
		t.Fatal(err)
	}
	return items[0]
}

func TestCreditBatchCreation(t *testing.T) {
	t.Parallel()
	e, _, items := creditEnv(t, "8100", "9999")
	if len(items) != 1 || items[0].WorkshopID != "8100" {
		t.Fatalf("failed ID recorded %+v", items)
	}
	for _, tc := range []struct{ cmd, batch, dry bool }{{true, false, false}, {true, true, true}, {false, true, false}} {
		steamDLL(e)
		var calls [][]string
		e.App.SteamWorker = fakeSteamWorker(&calls)
		if _, err := e.App.SubscribeWorkshopBatch([]string{"8200"}, tc.cmd, tc.batch, tc.dry); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.App.SubscribeWorkshopBatch([]string{"9999"}, true, true, false); err == nil {
		t.Fatal("all-failed call succeeded")
	}
	bs, _ := e.App.Store.CreditBatches("")
	if len(bs) != 1 {
		t.Fatalf("non-bulk/dry/Steam/all-failed created batch %+v", bs)
	}
}

func TestCreditSessionsAndMergedPrompt(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100", "8200")
	second, err := e.App.Store.CreateCreditBatch(e.Now(), items[:1])
	if err != nil {
		t.Fatal(err)
	}
	// Unrelated sessions do not count. Duplicates within and across calls count once.
	creditLaunch(t, e, "unrelated")
	launch := creditLaunch(t, e, items[0].InstanceID, items[1].InstanceID)
	if err := e.App.Store.CountCreditSession(launch, []string{items[0].InstanceID, items[1].InstanceID}); err != nil {
		t.Fatal(err)
	}
	bs, _ := e.App.Store.CreditBatches("")
	if len(bs) != 2 || bs[0].Sessions != 1 || bs[1].Sessions != 1 {
		t.Fatalf("duplicate session %+v", bs)
	}
	creditLaunch(t, e, items[1].InstanceID)
	bs, _ = e.App.Store.CreditBatches("")
	if bs[0].Sessions != 2 || bs[1].Sessions != 1 {
		t.Fatalf("wrong batch use %+v", bs)
	}
	creditLaunch(t, e, items[0].InstanceID)
	creditLaunch(t, e, items[0].InstanceID)
	// Steam unavailable defers without consuming either batch.
	p, err := e.App.CreditPrompt()
	if err != nil || p != nil {
		t.Fatalf("missing DLL %+v %v", p, err)
	}
	steamDLL(e)
	e.App.SteamWorker = func(string, []string) (string, error) { return "ERROR offline", nil }
	p, err = e.App.CreditPrompt()
	if err != nil || p != nil {
		t.Fatalf("offline %+v %v", p, err)
	}
	var calls [][]string
	e.App.SteamWorker = fakeSteamWorker(&calls)
	e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 1} }
	p, err = e.App.CreditPrompt()
	if err != nil || p != nil {
		t.Fatalf("running %+v %v", p, err)
	}
	e.App.GameProbe = func(string) *app.GameRun { return nil }
	for range 2 {
		p, err = e.App.CreditPrompt()
		if err != nil || p == nil || len(p.BatchIDs) != 2 || p.ModCount != 2 || len(p.Examples) != 2 {
			t.Fatalf("merged %+v %v", p, err)
		}
	}
	if err := e.App.CreditAnswer([]int64{batch, second}, false); err != nil {
		t.Fatal(err)
	}
	if err := e.App.CreditAnswer([]int64{batch}, true); err == nil {
		t.Fatal("dismissal reversed")
	}
	creditDue(t, e, items)
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	p, err = r.CreditPrompt()
	if err != nil || p != nil {
		t.Fatalf("dismissal did not persist %+v %v", p, err)
	}
	bs, _ = r.Store.CreditBatches("dismissed")
	if len(bs) != 2 {
		t.Fatal(bs)
	}
}

func TestCreditActualLaunchHook(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("enable", items[0].InstanceID)
	e.MustRun("launch", "--dry-run")
	bs, _ := e.App.Store.CreditBatches("")
	if bs[0].Sessions != 0 {
		t.Fatal(bs)
	}
	e.App.Launcher = func(string, []string, string) error { return errors.New("fake start failed") }
	if _, err := e.App.Launch(false); err == nil {
		t.Fatal("failed launch accepted")
	}
	bs, _ = e.App.Store.CreditBatches("")
	if bs[0].Sessions != 0 {
		t.Fatal(bs)
	}
	e.App.Launcher = func(string, []string, string) error { return nil }
	e.MustRun("launch")
	e.MustRun("disable", items[0].InstanceID)
	e.MustRun("launch")
	bs, _ = e.App.Store.CreditBatches("")
	if bs[0].ID != batch || bs[0].Sessions != 1 {
		t.Fatal(bs)
	}
	// Failing to count the session does not fail a launch that started.
	e.MustRun("enable", items[0].InstanceID)
	if _, err := e.App.Store.DB.Exec(`CREATE TRIGGER reject_credit_session BEFORE INSERT ON credit_session BEGIN SELECT RAISE(FAIL,'test count failure'); END`); err != nil {
		t.Fatal(err)
	}
	started := false
	e.App.Launcher = func(string, []string, string) error { started = true; return nil }
	if _, err := e.App.LaunchWith(false, false); err != nil || !started {
		t.Fatalf("successful launch reported failure: started=%t err=%v", started, err)
	}
}

func TestCreditReplacementResumeAndFailures(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100", "8200")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	calls := map[string]int{}
	installed := false
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		id := args[1]
		calls[args[0]+id]++
		if args[0] == steamworks.Subscribe {
			if id == "8200" {
				return "ITEM 8200 failed:15 0\nDONE subscribe 1", nil
			}
			return "ITEM 8100 ok 1\nDONE subscribe 1", nil
		}
		state := steamworks.ItemSubscribed | steamworks.ItemInstalled | steamworks.ItemNeedsUpdate
		if installed {
			state = steamworks.ItemSubscribed | steamworks.ItemInstalled
		}
		return fmt.Sprintf("STATE %s %d\nDONE state 1", id, state), nil
	}
	trashed := []string{}
	e.App.TrashFunc = func(path string) error { trashed = append(trashed, path); return creditRecycle(e, path) }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	its, _ := e.App.Store.CreditItems(batch)
	if its[0].State != "subscribed" || its[1].State != "failed" || len(trashed) != 0 {
		t.Fatalf("before install %+v %v", its, trashed)
	}
	if _, err := os.Stat(filepath.Join(e.App.SteamCMDModsDir(), "steamapps", "workshop", "content", "294100", "8200", "About", "About.xml")); err != nil {
		t.Fatal("failed subscription lost copy", err)
	}
	// Simulate closing and reopening with a fresh connection/cache after subscription.
	installed = true
	e.Advance(time.Minute)
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	its, _ = r.Store.CreditItems(batch)
	if its[0].State != "done" || its[1].State != "failed" || len(trashed) != 1 || !strings.Contains(trashed[0], "8100") {
		t.Fatalf("resume %+v trash %v", its, trashed)
	}
	bs, _ := r.Store.CreditBatches("done")
	if len(bs) != 1 {
		t.Fatal(bs)
	}
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	if calls["subscribe8100"] != 1 || calls["subscribe8200"] != 1 || len(trashed) != 1 {
		t.Fatalf("repeated work %v %v", calls, trashed)
	}
}

func TestCreditRetryPauseAndCancelBoundary(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100", "8200")
	creditStart(t, e, batch, items)
	steamDLL(e)
	calls := 0
	stop := false
	e.App.SteamWorker = func(string, []string) (string, error) {
		calls++
		stop = true
		return "ITEM 8100 failed:84 0\nDONE subscribe 1", nil
	}
	if err := e.App.CreditStep(nil, app.CreditCallbacks{Stopped: func() bool { return stop }}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("stop crossed item boundary", calls)
	}
	first := creditItem(t, e, batch)
	if first.State != "pending" || first.Attempts != 1 || first.NextAttempt <= e.Now().Unix() {
		t.Fatal(first)
	}
	// Another process sees the same backoff; a running game blocks every Steam call.
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("called Steam in game", calls)
	}
	r.GameProbe = func(string) *app.GameRun { return nil }
	e.Advance(time.Hour)
	r.SteamWorker = func(string, []string) (string, error) { return "ERROR Steam unavailable", nil }
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	first = creditItem(t, e, batch)
	if first.Attempts != 2 || first.State != "pending" {
		t.Fatal(first)
	}
}

func TestCreditReplacePlansRemapsHistoryAndPreservesUnrelatedLinks(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	otherDir := filepath.Join(e.App.SteamCMDModsDir(), "steamapps", "workshop", "content", "294100", "8200")
	e.WriteMod(otherDir, testkit.Sample("credit.mod8200", "Other batch"))
	scanNow(t, e)
	other := refID(t, e, "Other batch", source.SteamCMD)
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	e.MustRun("enable", items[0].InstanceID, other)
	e.MustRun("save")
	e.MustRun("disable", other) // An unrelated unsaved edit must not be deployed.
	p, err := e.App.NewPlan("second", "")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := e.App.Store.Workspace(p.Plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	ws.Entries = loadout.List{{Instance: items[0].InstanceID}}.Encode()
	if err := e.App.Store.SetWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	e.App.TrashFunc = func(path string) error { return creditRecycle(e, path) }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "done" {
		t.Fatalf("history must follow client copy %+v", it)
	}
	for _, plan := range mustCreditPlans(t, e) {
		ws, err := e.App.Store.Workspace(plan.ID)
		if err != nil {
			t.Fatal(err)
		}
		if loadout.Decode(ws.Entries).Index(items[0].InstanceID) >= 0 {
			t.Fatalf("old copy still in %s", plan.Name)
		}
	}
	mc := e.ReadModsConfig()
	if !strings.Contains(mc, "credit.mod8200") {
		t.Fatal("unrelated unsaved edit was written", mc)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", app.DeployName(mustCreditMod(t, e, other)))); err != nil {
		t.Fatal("unrelated link changed", err)
	}
	if !strings.Contains(mc, "credit.mod8100") || strings.Contains(mc, "credit.mod8100_steam") {
		t.Fatalf("wrong projected registry token %s", mc)
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", app.DeployName(mustCreditMod(t, e, items[0].InstanceID)))); !os.IsNotExist(err) {
		t.Fatal("old deployment link remains", err)
	}
}
func mustCreditPlans(t *testing.T, e *testkit.Env) []*store.Plan {
	t.Helper()
	p, err := e.App.Store.Plans()
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func mustCreditMod(t *testing.T, e *testkit.Env, id string) *store.Mod {
	t.Helper()
	m, err := e.App.Store.Mod(id)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	return m
}

func TestCreditSessionRequiresCurrentSteamCMD(t *testing.T) {
	t.Parallel()
	e, _, items := creditEnv(t, "8100")
	if _, err := e.App.Store.DB.Exec(`UPDATE instance SET status='tombstone' WHERE id=?`, items[0].InstanceID); err != nil {
		t.Fatal(err)
	}
	creditLaunch(t, e, items[0].InstanceID)
	bs, _ := e.App.Store.CreditBatches("")
	if bs[0].Sessions != 0 {
		t.Fatal(bs)
	}
	// Merely enabling a workshop instance of the same ID must not count either.
	e.AddWorkshop("8100", testkit.Sample("credit.mod8100", "Credit 8100"))
	scanNow(t, e)
	m := refID(t, e, "Credit 8100", source.Workshop)
	creditLaunch(t, e, m)
	bs, _ = e.App.Store.CreditBatches("")
	if bs[0].Sessions != 0 {
		t.Fatal(bs)
	}
}

func TestCreditForeignLinkRetainsCopy(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	old := mustCreditMod(t, e, items[0].InstanceID)
	link := filepath.Join(e.Game, "Mods", "ForeignCredit")
	if err := source.CreateLink(link, old.RealPath); err != nil {
		t.Fatal(err)
	}
	e.App.TrashFunc = func(string) error { t.Fatal("linked copy recycled"); return nil }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "retained" || !strings.Contains(it.Result, "links") {
		t.Fatalf("foreign link %+v", it)
	}
	if !source.IsLink(link) {
		t.Fatal("foreign link changed")
	}
}

func TestCreditInterruptedRecycleResumes(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	calls := 0
	e.App.TrashFunc = func(string) error { calls++; return errors.New("fake recycle failure") }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "switching" || it.TargetID == "" || calls != 1 {
		t.Fatalf("lost switch checkpoint %+v %d", it, calls)
	}
	e.Advance(time.Hour)
	e.App.TrashFunc = func(path string) error {
		calls++
		// Recycling can be slow. A separate edit session must already be able
		// to acquire the launch guard, including when resuming a checkpoint.
		r, err := e.App.ReadSession()
		if err != nil {
			return err
		}
		defer r.Close()
		guard := make(chan error, 1)
		go func() {
			_, err := r.MoveBlock(nil, 1) // takes the launch lock, writes nothing
			guard <- err
		}()
		if err := <-guard; err != nil {
			return err
		}
		return creditRecycle(e, path)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it = creditItem(t, e, batch)
	if it.State != "done" || calls != 2 {
		t.Fatalf("did not resume switch %+v %d", it, calls)
	}
}

func TestCreditDeploymentsAcrossGamesResume(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	e.App.TrashFunc = func(path string) error { return creditRecycle(e, path) }
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	e.MustRun("enable", items[0].InstanceID)
	e.MustRun("save")
	game := secondGame(e, "CreditSecond")
	e.MustRun("env", "add", "credit-second", "--game", game, "--saves", "own")
	e.MustRun("env", "use", "credit-second")
	e.MustRun("enable", items[0].InstanceID)
	e.MustRun("save")
	e.MustRun("env", "use", "main")
	// A checkpoint with one plan already switched but both projections outstanding
	// models an exit in the middle of the all-environment handoff.
	e.MustRun("scan")
	old := mustCreditMod(t, e, items[0].InstanceID)
	target := refID(t, e, "Credit 8100", source.Workshop)
	en, _ := e.App.Store.Envs()
	for _, env := range en {
		if err := e.App.Store.AddCreditDeployment(items[0], env.ID, e.App.Store.CurrentPlanID(env.ID)); err != nil {
			t.Fatal(err)
		}
	}
	it := items[0]
	it.State = "switching"
	it.TargetID = target
	if err := e.App.Store.SaveCreditItem(it); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Swap(old.ID, target); err != nil {
		t.Fatal(err)
	}
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it = creditItem(t, e, batch)
	if it.State != "done" {
		t.Fatalf("did not finish deployment resume %+v", it)
	}
	if ds, err := r.Store.CreditDeployments(it); err != nil || len(ds) != 0 {
		t.Fatalf("unfinished deployments %+v %v", ds, err)
	}
	for _, env := range en {
		b, err := os.ReadFile(filepath.Join(env.UserDataRoot, "Config", "ModsConfig.xml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "credit.mod8100") || strings.Contains(string(b), "credit.mod8100_steam") {
			t.Fatalf("bad projection %s: %s", env.Name, b)
		}
		if _, err := os.Lstat(filepath.Join(env.GameRoot, "Mods", app.DeployName(old))); !os.IsNotExist(err) {
			t.Fatalf("link remains %s: %v", env.Name, err)
		}
	}
}

func TestCreditDoesNotSwitchWhenGameStartsDuringSteamCall(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	e.MustRun("enable", items[0].InstanceID)
	before, _ := e.App.Store.Workspace(mustPlan(t, e))
	e.App.SteamWorker = func(_ string, args []string) (string, error) {
		if args[0] == "subscribe" {
			return "ITEM 8100 ok 1\nDONE subscribe 1", nil
		}
		e.App.GameProbe = func(string) *app.GameRun { return &app.GameRun{PID: 42} }
		return "STATE 8100 5\nDONE state 1", nil
	}
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	after, _ := e.App.Store.Workspace(before.PlanID)
	if after.Entries != before.Entries {
		t.Fatal("plan changed while game was running")
	}
	it := creditItem(t, e, batch)
	if it.State != "subscribed" || !strings.Contains(it.Result, "running") {
		t.Fatal(it)
	}
}

func TestCreditStageAndSteamSuffix(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	e.App.TrashFunc = func(path string) error { return creditRecycle(e, path) }
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	// A separate local copy is not in the batch and remains installed. The client
	// copy must therefore be projected with _steam even after the CMD link is gone.
	e.MustRun("enable", items[0].InstanceID)
	e.MustRun("save")
	e.WriteMod(filepath.Join(e.Game, "Mods", "Independent"), testkit.Sample("credit.mod8100", "Independent copy"))
	e.MustRun("scan")
	plans := mustCreditPlans(t, e)
	stagingPlan, err := e.App.Store.CreatePlan(plans[0].EnvID, "staging-credit", e.Now())
	if err != nil {
		t.Fatal(err)
	}
	raw := fmt.Sprintf(`[{"i":%q,"p":2},{"i":"unrelated","a":%q,"p":3}]`, items[0].InstanceID, items[0].InstanceID)
	if err := e.App.Store.SetMeta(store.StageKey(stagingPlan.ID), raw); err != nil {
		t.Fatal(err)
	}
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "done" || it.TargetID == "" {
		t.Fatalf("switch %+v", it)
	}
	var staged []app.StageItem
	raw = e.App.Store.Meta(store.StageKey(stagingPlan.ID))
	if err := json.Unmarshal([]byte(raw), &staged); err != nil {
		t.Fatal(err)
	}
	if staged[0].Instance != it.TargetID || staged[1].After != it.TargetID || staged[0].Pos != 2 || staged[1].Instance != "unrelated" {
		t.Fatalf("stage changed incorrectly %+v", staged)
	}
	if !strings.Contains(e.ReadModsConfig(), "credit.mod8100_steam") {
		t.Fatal("client copy token lost its Steam suffix", e.ReadModsConfig())
	}
	if _, err := os.Stat(filepath.Join(e.Game, "Mods", "Independent", "About", "About.xml")); err != nil {
		t.Fatal("non-batch copy changed", err)
	}
}

func TestCreditWaitsForScannedClientAndReadableAbout(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	creditStart(t, e, batch, items)
	steamDLL(e)
	var calls [][]string
	e.App.SteamWorker = fakeSteamWorker(&calls)
	e.App.TrashFunc = func(string) error { t.Fatal("recycled before client is ready"); return nil }
	// Even an Installed bit does not establish a usable client instance.
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "subscribed" || !strings.Contains(it.Result, "About.xml") {
		t.Fatal(it)
	}
	e.AddWorkshop("8100", testkit.Sample("credit.mod8100", "Credit 8100"))
	if err := os.Remove(filepath.Join(e.Workshop, "8100", "About", "About.xml")); err != nil {
		t.Fatal(err)
	}
	e.Advance(time.Hour)
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it = creditItem(t, e, batch)
	if it.State != "subscribed" || !strings.Contains(it.Result, "About.xml") {
		t.Fatal(it)
	}
}

func TestCreditSessionsSurviveRemovingGame(t *testing.T) {
	t.Parallel()
	e, _, items := creditEnv(t, "8100")
	en, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	creditLaunch(t, e, items[0].InstanceID)
	if err := e.App.Store.DeleteEnv(en.ID); err != nil {
		t.Fatal("credit session blocked removing game", err)
	}
	// Old counted sessions belong to the batch, even if their game and launch log
	// have been removed. Reused SQLite launch row IDs in another game are distinct.
	game := secondGame(e, "CreditAfterRemoval")
	e.MustRun("env", "add", "after-credit", "--game", game, "--saves", "own")
	e.MustRun("env", "use", "after-credit")
	creditLaunch(t, e, items[0].InstanceID)
	bs, err := e.App.Store.CreditBatches("")
	if err != nil || len(bs) != 1 || bs[0].Sessions != 2 {
		t.Fatalf("lost or conflated sessions %+v %v", bs, err)
	}
}

func TestCreditRealLaunchRemapsRestorableStatesAndRecycles(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	old := items[0].InstanceID
	e.MustRun("env", "set", "main", "--launch", "direct")
	e.MustRun("enable", old)
	for range 3 {
		e.MustRun("launch")
	}
	bs, err := e.App.Store.CreditBatches("prompt")
	if err != nil || len(bs) != 1 || bs[0].Sessions != 3 {
		t.Fatalf("real launches: %+v %v", bs, err)
	}
	plan := mustPlan(t, e)
	versions, _ := e.App.Store.Versions(plan)
	hasOld := false
	for _, v := range versions {
		hasOld = hasOld || loadout.Decode(v.Entries).Index(old) >= 0
	}
	if !hasOld {
		t.Fatal("launch did not write the old identity into history")
	}
	// An ordinary edit leaves an undo entry that contains the SteamCMD instance.
	e.WriteMod(filepath.Join(e.Game, "Mods", "Unrelated"), testkit.Sample("credit.unrelated", "Unrelated"))
	e.MustRun("scan")
	e.MustRun("enable", "pkg:credit.unrelated")
	e.MustRun("disable", "pkg:credit.unrelated")
	if err := e.App.Store.PushStep(plan, "redo", loadout.List{{Instance: old}}.Encode(), "fixture"); err != nil {
		t.Fatal(err)
	}
	creditClient(t, e, items)
	if err := e.App.CreditAnswer([]int64{batch}, true); err != nil {
		t.Fatal(err)
	}
	original := mustCreditMod(t, e, old).RealPath
	recycled := ""
	e.App.TrashFunc = func(path string) error {
		recycled = path
		r, err := e.App.ReadSession()
		if err != nil {
			return err
		}
		defer r.Close()
		guard := make(chan error, 1)
		go func() {
			_, err := r.MoveBlock(nil, 1) // takes the launch lock, writes nothing
			guard <- err
		}()
		if err := <-guard; err != nil {
			return err
		}
		return creditRecycle(e, path)
	}
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "done" || recycled != filepath.Join(filepath.Dir(original), ".archo-credit-"+it.WorkshopID+"-"+it.InstanceID) {
		t.Fatalf("real used copy retained: %+v trash=%q", it, recycled)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("old folder still exists: %v", err)
	}
	for _, table := range []string{"workspace", "workspace_undo", "loadout_version"} {
		rows, err := e.App.Store.DB.Query(`SELECT entries FROM `+table+` WHERE plan_id=?`, plan)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if loadout.Decode(raw).Index(old) >= 0 {
				t.Fatalf("old identity in %s: %s", table, raw)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	// Undo after the physical copy is gone still resolves to the client copy.
	e.MustRun("undo")
	st, err := e.App.Loadout()
	if err != nil || st.List.Index(it.TargetID) < 0 || st.List.Index(old) >= 0 || !st.Reg.Loadable(it.TargetID) {
		t.Fatalf("undo restored unavailable copy: %+v %v", st, err)
	}
}

func TestCreditCrossEnvironmentCandidateRequired(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	old := items[0].InstanceID
	e.MustRun("enable", old)
	shared := mustPlan(t, e)
	mainEnv, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	game := secondGame(e, "CreditBlocked")
	e.MustRun("env", "add", "blocked", "--game", game, "--saves", "own")
	e.MustRun("env", "use", "blocked")
	blockedEnv, err := e.App.Env()
	if err != nil {
		t.Fatal(err)
	}
	// B uses the same list but has never saved or launched it. Its ModsConfig
	// contains no old copy and the list's EnvID still names A.
	if err := e.App.Store.SetCurrentPlan(blockedEnv.ID, shared); err != nil {
		t.Fatal(err)
	}
	e.MustRun("env", "use", "main")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	var target *store.Mod
	for _, m := range ix.Mods {
		if m.Kind == source.Workshop && m.WorkshopID() == "8100" {
			target = m
		}
	}
	if target == nil {
		t.Fatal("missing client copy")
	}
	// A sees the client copy, B does not. Scan preserves source ownership.
	if _, err := e.App.Store.DB.Exec(`UPDATE source SET environment_id=? WHERE id=?`, mainEnv.ID, target.SourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.App.Store.DB.Exec(`UPDATE instance SET environment_id=? WHERE id=?`, mainEnv.ID, target.ID); err != nil {
		t.Fatal(err)
	}
	e.App.TrashFunc = func(string) error { t.Fatal("copy needed by blocked game recycled"); return nil }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "retained" || !strings.Contains(it.Result, "blocked") {
		t.Fatalf("missing environment reason: %+v", it)
	}
	ws, err := e.App.Store.Workspace(shared)
	if err != nil || loadout.Decode(ws.Entries).Index(old) < 0 || loadout.Decode(ws.Entries).Index(target.ID) >= 0 {
		t.Fatalf("shared list switched: %+v %v", ws, err)
	}
	if ds, err := e.App.Store.CreditDeployments(it); err != nil || len(ds) != 0 {
		t.Fatalf("handoff started before eligibility: %+v %v", ds, err)
	}
}

func TestCreditOverlappingBatchesSubscribeOnce(t *testing.T) {
	for _, installed := range []bool{false, true} {
		t.Run(fmt.Sprint(installed), func(t *testing.T) {
			t.Parallel()
			e, first, items := creditEnv(t, "8100")
			second, err := e.App.Store.CreateCreditBatch(e.Now(), items)
			if err != nil {
				t.Fatal(err)
			}
			creditDue(t, e, items)
			if err := e.App.CreditAnswer([]int64{first, second}, true); err != nil {
				t.Fatal(err)
			}
			creditClient(t, e, items)
			calls := 0
			e.App.SteamWorker = func(_ string, args []string) (string, error) {
				if args[0] == steamworks.Subscribe {
					calls++
					return "ITEM 8100 ok 1\nDONE subscribe 1", nil
				}
				state := steamworks.ItemSubscribed
				if installed {
					state |= steamworks.ItemInstalled
				}
				return fmt.Sprintf("STATE 8100 %d\nDONE state 1", state), nil
			}
			e.App.TrashFunc = func(path string) error { return creditRecycle(e, path) }
			if err := e.App.CreditStep(nil); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("repeated subscription: %d", calls)
			}
			a, b := creditItem(t, e, first), creditItem(t, e, second)
			if !installed {
				if a.State != "subscribed" || b.State != "subscribed" {
					t.Fatalf("confirmation not shared: %+v %+v", a, b)
				}
				installed = true
				e.Advance(time.Minute)
				// Durable confirmation must survive a new connection and both pending batches.
				r, err := e.App.ReadSession()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				if err := r.CreditStep(nil); err != nil {
					t.Fatal(err)
				}
				b = creditItem(t, e, second)
			}
			if b.State != "done" || b.Result != "already replaced with Workshop" || calls != 1 {
				t.Fatalf("overlap: %+v calls=%d", b, calls)
			}
		})
	}
}

func TestCreditReadinessWaitKeepsAttempts(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	for _, reason := range []string{"waiting for scan", "waiting for file task"} {
		for range 12 {
			if err := e.App.CreditStep(func(store.CreditItem) (string, error) { return "", app.CreditWait(reason) }); err != nil {
				t.Fatal(err)
			}
			it := creditItem(t, e, batch)
			if it.Attempts != 0 || it.NextAttempt-e.Now().Unix() != 5 || it.Result != reason {
				t.Fatalf("wait became failure: %+v", it)
			}
			e.Advance(5 * time.Second)
		}
	}
}

func TestCreditRemapRollsBackAllListStates(t *testing.T) {
	t.Parallel()
	e, _, items := creditEnv(t, "8100")
	creditClient(t, e, items)
	e.MustRun("enable", items[0].InstanceID)
	e.MustRun("save")
	e.MustRun("scan")
	p := mustPlan(t, e)
	target := refID(t, e, "Credit 8100", source.Workshop)
	before, _ := e.App.Store.Workspace(p)
	// Failure in staging comes after writes to workspace/history/undo in the transaction.
	if err := e.App.Store.SetMeta(store.StageKey(p), "broken JSON"); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.RemapCreditPlans([]int64{p}, items[0].InstanceID, target, e.Now()); err == nil {
		t.Fatal("malformed stage accepted")
	}
	after, _ := e.App.Store.Workspace(p)
	if after.Entries != before.Entries {
		t.Fatal("partial workspace remap committed")
	}
	versions, _ := e.App.Store.Versions(p)
	if loadout.Decode(versions[0].Entries).Index(items[0].InstanceID) < 0 {
		t.Fatal("partial version remap committed")
	}
}

func TestCreditOverlappingBatchesSharePermanentFailure(t *testing.T) {
	t.Parallel()
	e, first, items := creditEnv(t, "8100")
	second, err := e.App.Store.CreateCreditBatch(e.Now(), items)
	if err != nil {
		t.Fatal(err)
	}
	creditDue(t, e, items)
	if err := e.App.CreditAnswer([]int64{first, second}, true); err != nil {
		t.Fatal(err)
	}
	creditClient(t, e, items)
	calls := 0
	e.App.SteamWorker = func(string, []string) (string, error) { calls++; return "ITEM 8100 failed:15 0\nDONE subscribe 1", nil }
	e.App.TrashFunc = func(string) error { t.Fatal("failed subscription recycled"); return nil }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	a, b := creditItem(t, e, first), creditItem(t, e, second)
	if calls != 1 || a.State != "failed" || b.State != "failed" || a.Result != b.Result {
		t.Fatalf("failure not shared: %+v %+v calls=%d", a, b, calls)
	}
	if _, err := os.Stat(mustCreditMod(t, e, items[0].InstanceID).RealPath); err != nil {
		t.Fatal("lost retained copy", err)
	}
}

func TestCreditResumeAfterRecycleBeforeScan(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	e.MustRun("enable", items[0].InstanceID)
	e.MustRun("save")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	calls := 0
	e.App.TrashFunc = func(path string) error {
		calls++
		if err := creditRecycle(e, path); err != nil {
			return err
		}
		return errors.New("exit after OS recycle, before scan/checkpoint")
	}
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "switching" {
		t.Fatal(it)
	}
	e.Advance(time.Hour)
	r, err := e.App.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it = creditItem(t, e, batch)
	if it.State != "done" || calls != 1 {
		t.Fatalf("recycled folder retried: %+v calls=%d", it, calls)
	}
}

func TestCreditNotesAndTextures(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	old := items[0].InstanceID
	e.MustRun("enable", old)
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	e.MustRun("scan")
	ix, err := e.App.Index()
	if err != nil {
		t.Fatal(err)
	}
	var target string
	for _, m := range ix.Mods {
		if m.Kind == source.Workshop && m.WorkshopID() == "8100" {
			target = m.ID
		}
	}
	if target == "" {
		t.Fatal("missing Workshop")
	}
	for id, text := range map[string]string{old: "old note", target: "client note"} {
		if err := e.App.Store.SetCopyNote(id, text, e.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.App.Store.SetMade(old, &store.Made{}); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.PutTextureMod(store.TextureMod{InstanceID: old, SnapHash: []byte("old-content"), Scale: 50, MadeAt: e.Now()}); err != nil {
		t.Fatal(err)
	}
	folder := mustCreditMod(t, e, old).RealPath
	dds := filepath.Join(folder, "Textures", "test.dds")
	if err := e.App.Store.PutTextureFiles([]store.TextureFile{{Path: dds, InstanceID: old, ModDir: folder, Rel: "Textures/test.dds", Scale: 50, MadeAt: e.Now()}}); err != nil {
		t.Fatal(err)
	}
	e.App.TrashFunc = func(path string) error { return creditRecycle(e, path) }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "done" || !strings.Contains(it.Result, "optimise Workshop textures again") {
		t.Fatalf("texture warning lost: %+v", it)
	}
	var text, made string
	if err := e.App.Store.DB.QueryRow(`SELECT text,made FROM instance_note WHERE instance_id=?`, target).Scan(&text, &made); err != nil {
		t.Fatal(err)
	}
	if text != "client note\n\nold note" || made == "" {
		t.Fatalf("note merge: %q %q", text, made)
	}
	var n int
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM instance_note WHERE instance_id=?`, old).Scan(&n); err != nil || n != 0 {
		t.Fatalf("old note: %d %v", n, err)
	}
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM texture_file WHERE instance_id=? AND path=?`, old, dds).Scan(&n); err != nil || n != 1 {
		t.Fatalf("physical ledger relabelled: %d %v", n, err)
	}
	if err := e.App.Store.DB.QueryRow(`SELECT count(*) FROM texture_mod WHERE instance_id=?`, target).Scan(&n); err != nil || n != 0 {
		t.Fatalf("false optimisation state: %d %v", n, err)
	}
	ws, err := e.App.Store.Workspace(mustPlan(t, e))
	if err != nil || loadout.Decode(ws.Entries).Index(old) >= 0 || loadout.Decode(ws.Entries).Index(target) < 0 {
		t.Fatalf("old copy resurrected: %+v %v", ws, err)
	}
}

func TestCreditMalformedStagingRetained(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	old := items[0].InstanceID
	e.MustRun("enable", old)
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	if err := e.App.Store.SetMeta(store.StageKey(mustPlan(t, e)), `[{broken`); err != nil {
		t.Fatal(err)
	}
	e.App.TrashFunc = func(string) error { t.Fatal("corrupt staging recycled"); return nil }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "retained" || !strings.Contains(it.Result, "staging list") || it.Attempts != 0 {
		t.Fatalf("corruption retried: %+v", it)
	}
	ws, err := e.App.Store.Workspace(mustPlan(t, e))
	if err != nil || loadout.Decode(ws.Entries).Index(old) < 0 {
		t.Fatalf("list mutated: %+v %v", ws, err)
	}
	e.Advance(time.Hour)
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	if again := creditItem(t, e, batch); again != it {
		t.Fatalf("terminal item retried: %+v", again)
	}
}

func TestCreditManagedLinkOutsideEnvironmentRetained(t *testing.T) {
	t.Parallel()
	e, batch, items := creditEnv(t, "8100")
	creditStart(t, e, batch, items)
	creditClient(t, e, items)
	old := mustCreditMod(t, e, items[0].InstanceID)
	mods := filepath.Join(e.Root, "removed-game", "Mods")
	e.MkdirAll(mods)
	name := app.DeployPrefix + "credit"
	link := filepath.Join(mods, name)
	if err := source.CreateLink(link, old.RealPath); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.RecordLinkIntent([]store.ManagedLink{{Operation: "credit-test", Mods: mods, Name: name, Kind: "deploy", Instance: old.ID, Target: old.RealPath}}); err != nil {
		t.Fatal(err)
	}
	if err := e.App.Store.LinkPhase("credit-test", "complete"); err != nil {
		t.Fatal(err)
	}
	e.App.TrashFunc = func(string) error { t.Fatal("managed link target recycled"); return nil }
	if err := e.App.CreditStep(nil); err != nil {
		t.Fatal(err)
	}
	it := creditItem(t, e, batch)
	if it.State != "retained" || !strings.Contains(it.Result, "links") {
		t.Fatalf("managed link ignored: %+v", it)
	}
	if !source.IsLink(link) {
		t.Fatal("managed link changed")
	}
}
