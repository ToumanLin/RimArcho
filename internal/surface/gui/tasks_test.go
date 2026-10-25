package gui

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/view"
)

func endedTask(t *testing.T, api *API, kind string) view.TaskRow {
	t.Helper()
	for i := 0; i < 2000; i++ {
		for _, r := range api.Tasks().Rows {
			if r.Kind == kind && r.Group != "run" && r.Group != "queue" {
				return r
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %s task ended", kind)
	return view.TaskRow{}
}

// Mod 库's actions on folders are background tasks (06 §2.16): queued, one item at a time,
// one failure does not stop the rest, every item keeps its result.
func TestLibraryActionIsATask(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	// neither copy is hidden: unhide fails on each, and says why
	text, err := api.LibraryDo(view.LibUnhide, []string{"pkg:tester.b", "no.such.mod"})
	if err != nil || !strings.Contains(text, "queue") && !strings.Contains(text, "队列") {
		t.Fatalf("submit: %q %v", text, err)
	}
	r := endedTask(t, api, view.LibUnhide)
	if r.State != app.TaskFailed || len(r.Items) != 2 {
		t.Fatalf("task %+v", r)
	}
	for _, it := range r.Items {
		if it.State != app.ItemFail || it.Why == "" {
			t.Fatalf("item %+v", it)
		}
	}
	if !hasAct(r, "retry") || !hasAct(r, "forget") || hasAct(r, "stop") {
		t.Fatalf("acts %+v", r.Acts)
	}
	if _, err := api.TaskAct("retry", []string{r.ID}, r.Items[0].Ref); err != nil {
		t.Fatal(err)
	}
	if n := len(api.Tasks().Rows); n != 2 {
		t.Fatalf("%d tasks after a retry of one item", n)
	}
	for i := 0; i < 2000; i++ {
		if p := api.Tasks(); p.Running+p.Queued == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if p := api.Tasks(); p.Unseen != 1 { // the retry failed again; the first one was looked at by retrying it
		for _, x := range p.Rows {
			t.Logf("%s %s %s seen=%v %s", x.ID, x.Kind, x.State, x.Seen, x.Line)
		}
		t.Fatalf("%d failures are news", p.Unseen)
	}
	api.TasksSeen()
	if _, err := api.TaskAct("forget", []string{r.ID}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := api.TaskAct("undo", nil, ""); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range api.Tasks().Rows {
		found = found || x.ID == r.ID
	}
	if !found {
		t.Fatal("undo did not put the task back")
	}
}

// The window's own jobs are recorded too: a scan that cannot start is a failed task.
func TestScanIsATask(t *testing.T) {
	t.Parallel()
	_, api, _ := fixture(t)
	api.Exe = t.TempDir() + "/none.exe"
	api.Scan()
	r := endedTask(t, api, view.TaskScan)
	if r.State != app.TaskFailed || r.Line == "" || hasAct(r, "stop") || !hasAct(r, "retry") {
		t.Fatalf("scan task %+v", r)
	}
}

func hasAct(r view.TaskRow, id string) bool {
	for _, a := range r.Acts {
		if a.ID == id {
			return true
		}
	}
	return false
}

// 移到回收站 runs as one batch (06 §2.16): every item checked, the folders moved together,
// one failure kept on its item, and the library rescanned once at the end.
func TestTrashBatch(t *testing.T) {
	t.Parallel()
	e, api, _ := fixture(t)
	var mu sync.Mutex
	var moved []string
	e.App.TrashFunc = func(p string) error {
		mu.Lock()
		defer mu.Unlock()
		moved = append(moved, p)
		return os.RemoveAll(p)
	}
	if _, err := api.LibraryDo(view.LibDelete, []string{"pkg:tester.b", "no.such.mod"}); err != nil {
		t.Fatal(err)
	}
	r := endedTask(t, api, view.LibDelete)
	if r.State != app.TaskPartial || len(moved) != 1 {
		t.Fatalf("state %s, moved %v", r.State, moved)
	}
	if r.Items[0].State != app.ItemOK || r.Items[1].State != app.ItemFail || r.Items[1].Why == "" {
		t.Fatalf("items %+v", r.Items)
	}
	// the scan after the batch: the mod is gone from the library
	st, err := e.App.Loadout()
	if err != nil {
		t.Fatal(err)
	}
	if m, err := st.Ix.ResolveOne("pkg:tester.b"); err == nil && m.Status == "live" {
		t.Fatal("the library was not rescanned")
	}
}
