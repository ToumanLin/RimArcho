package gui

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/testkit"
	"github.com/ToumanLin/RimArcho/internal/view"
)

// savesDrain reads the fixture's events for the Saves page tests: listings ("saves") go to
// lists, the full read's news to news; the rest (task changes) is dropped. It reads until
// the test binary ends, so a window closing late never blocks on a full channel.
func savesDrain(events chan string) (chan string, chan SavesReadEvent) {
	lists, news := make(chan string, 64), make(chan SavesReadEvent, 256)
	go func() {
		for ev := range events {
			switch {
			case strings.HasPrefix(ev, "saves "):
				lists <- ev
			case strings.HasPrefix(ev, "saves-read "):
				var r SavesReadEvent
				if json.Unmarshal([]byte(strings.TrimPrefix(ev, "saves-read ")), &r) == nil {
					select {
					case news <- r:
					default:
					}
				}
			}
		}
	}()
	return lists, news
}

// recordsXML is a history recorder's values as the game writes them (base64 of
// little-endian float32s).
func recordsXML(def string, v []float32) string {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return "<li><def>" + def + "</def><records>" + base64.StdEncoding.EncodeToString(b) + "</records></li>"
}

// playBody is the part of a synthetic save the right card reads (12 §1.7 L1): storyteller,
// play time, raids, the history series, letters, research and pawns of the player
// faction (Faction_1, the testkit's PlayerColony).
func playBody(wealth, people []float32, alive, dead []string, animals int, letters [][3]string) string {
	var b strings.Builder
	b.WriteString(`<info><realPlayTimeInteracting>12300</realPlayTimeInteracting></info>`)
	b.WriteString(`<storyteller><def>Cassandra</def><difficulty>Rough</difficulty></storyteller>`)
	b.WriteString(`<storyWatcher><statsRecord><numRaidsEnemy>4</numRaidsEnemy></statsRecord></storyWatcher>`)
	b.WriteString(`<researchManager><progress><keys><li>Electricity</li></keys><values><li>1600</li></values></progress></researchManager>`)
	b.WriteString(`<history><archive><archivables>`)
	for i, l := range letters {
		fmt.Fprintf(&b, `<li Class="StandardLetter"><ID>%d</ID><def>%s</def><label>%s</label><arrivalTick>%s</arrivalTick></li>`, i+1, l[1], l[2], l[0])
	}
	b.WriteString(`</archivables></archive><autoRecorderGroups><li><def>Wealth</def><recorders>`)
	b.WriteString(recordsXML("Wealth_Total", wealth))
	b.WriteString(`</recorders></li><li><def>Population</def><recorders>`)
	b.WriteString(recordsXML("FreeColonists", people))
	b.WriteString(`</recorders></li></autoRecorderGroups></history>`)
	b.WriteString(`<maps><li><uniqueID>0</uniqueID><things>`)
	n := 10
	pawn := func(name string, isDead bool) {
		n++
		health := ""
		if isDead {
			health = "<healthTracker><healthState>Dead</healthState></healthTracker>"
		}
		fmt.Fprintf(&b, `<thing Class="Pawn"><def>Human</def><id>Human%d</id><faction>Faction_1</faction><kindDef>Colonist</kindDef><name Class="NameTriple"><first>%s</first><nick>%s</nick><last>Lee</last></name><story><childhood>Urbworld</childhood></story>%s</thing>`, n, name, name, health)
	}
	for _, c := range alive {
		pawn(c, false)
	}
	for _, c := range dead {
		pawn(c, true)
	}
	for i := range animals {
		fmt.Fprintf(&b, `<thing Class="Pawn"><def>Husky</def><id>Husky%d</id><faction>Faction_1</faction><kindDef>Husky</kindDef><story IsNull="True" /><training><learned /></training></thing>`, 50+i)
	}
	b.WriteString(`</things></li></maps>`)
	return b.String()
}

// awaitNews waits for the full read's event that ok accepts.
func awaitNews(t *testing.T, news chan SavesReadEvent, what string, ok func(SavesReadEvent) bool) SavesReadEvent {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case ev := <-news:
			if ok(ev) {
				return ev
			}
		case <-deadline:
			t.Fatalf("no saves-read event: %s", what)
		}
	}
}

func savesTask(api *API) *app.Task {
	for _, t := range api.tasks.List() {
		if t.Kind == view.TaskSaves {
			return &t
		}
	}
	return nil
}

// The Saves page's full read (16 §2.11 "Loading", "Right"; 12 §1.7, §4.5): the listing
// starts the background task, its news relists the page (days, colonies by world), the
// selected save's right card fills in, and a save selected while the run is busy elsewhere
// is read on its own without waiting.
func TestSavesFullReadPage(t *testing.T) {
	e, api, events := fixture(t)
	lists, news := savesDrain(events)
	now := e.Now()
	spec := testkit.SaveSpec{Mods: []string{"ludeon.rimworld", "tester.a"}, World: "Ennawrin", Seed: "hollow", PRV: 7, Faction: "New Arrivals"}
	w1 := []float32{1000, 3000, 5000, 9000}
	spec.Ticks, spec.ModTime = 90000, now.Add(-3*time.Hour) // day 1.5
	spec.Body = playBody(w1, []float32{3, 3, 3, 3}, []string{"Mara", "Kari", "Ozzie"}, nil, 1, nil)
	e.WriteSave("Landing.rws", spec)
	w2 := []float32{1000, 3000, 5000, 9000, 12000, 15000, 20000}
	spec.Ticks, spec.ModTime, spec.Faction = 180000, now.Add(-2*time.Hour), "Iron Pact" // day 3
	spec.Body = playBody(w2, []float32{3, 3, 3, 3, 4, 4, 3}, []string{"Mara", "Kari", "Pike"}, []string{"Ozzie"}, 3,
		[][3]string{{"150000", "Death", "Death: Ozzie"}, {"120000", "ThreatBig", "Raid"}})
	e.WriteSave("Ennawrin.rws", spec) // another file name, the same world
	read := func(n uint64, first string) *view.SavesPage {
		t.Helper()
		api.SavesRead(n, first)
		select {
		case ev := <-lists:
			var got SavesEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(ev, "saves ")), &got); err != nil || got.Request != n || got.Page == nil {
				t.Fatalf("listing %d: %s", n, ev)
			}
			return got.Page
		case <-time.After(30 * time.Second):
			t.Fatal("no saves event")
		}
		return nil
	}
	rows := func(p *view.SavesPage) []string {
		var out []string
		for _, c := range p.Colonies {
			for _, s := range c.Saves {
				out = append(out, c.Name+"/"+s.Name+" "+s.When)
			}
		}
		return out
	}

	// Cold: rows show dates, colonies by file name; the run reads both and relists.
	p := read(1, "")
	if got := rows(p); len(got) != 2 || strings.Contains(strings.Join(got, ","), "day ") {
		t.Fatalf("cold rows %q", got)
	}
	end := awaitNews(t, news, "the run's end", func(ev SavesReadEvent) bool { return ev.Done })
	if end.Text != "read 2 saves" || end.Error != "" || end.Page == nil {
		t.Fatalf("end %+v", end)
	}
	if got, want := rows(end.Page), []string{"Iron Pact/Ennawrin day 3", "Iron Pact/Landing day 1"}; !slices.Equal(got, want) {
		t.Fatalf("relisted rows %q, want %q", got, want)
	}
	if tk := savesTask(api); tk == nil || tk.State != app.TaskDone || tk.Result != "read 2 saves" || tk.Total != 2 || tk.Obj != "2 saves" {
		t.Fatalf("task %+v", tk)
	}
	if r := end.Page.Colonies[0].Saves[0]; r.Saved == "" {
		t.Fatalf("the read row keeps no date for its hover: %+v", r)
	}

	// The right card of the newest save, against the previous one.
	d, err := api.SaveDetail("Ennawrin")
	if err != nil {
		t.Fatal(err)
	}
	pl := d.Play
	if pl == nil || d.Reading || d.PlayNote != "" {
		t.Fatalf("detail play %+v reading %v note %q", pl, d.Reading, d.PlayNote)
	}
	if pl.Day != "day 3" || pl.Date == "" || pl.Storyteller != "Cassandra Classic" || pl.Difficulty != "strive to survive" ||
		pl.Played != "3h 25m" || pl.Since != "Landing" {
		t.Fatalf("play head %+v", pl)
	}
	stats := map[string]string{}
	for _, st := range pl.Stats {
		stats[st.Label] = st.Value + " " + st.Change
	}
	if stats["wealth"] != "20k +11k" || stats["colonists"] != "3 ±0" || stats["animals"] != "3 +2" || stats["raids"] != "4 ±0" {
		t.Fatalf("stats %v", stats)
	}
	var people []string
	for _, c := range pl.Colonists {
		people = append(people, fmt.Sprintf("%s%v", c.Name, c.Dead))
	}
	if !slices.Equal(people, []string{"Marafalse", "Karifalse", "Pikefalse", "Ozzietrue"}) {
		t.Fatalf("colonists %q", people)
	}
	if len(pl.Events) != 2 || pl.Events[0].Text != "Death: Ozzie" || !pl.Events[0].Death || pl.Events[0].Day != 2 {
		t.Fatalf("events %+v", pl.Events)
	}
	tl := pl.Timeline
	if tl == nil || tl.Days != 3 || len(tl.Wealth) != 7 || len(tl.Points) != 2 || tl.Points[0].Day != 1 || tl.Points[1].Wealth != 20000 ||
		!slices.Equal(tl.Deaths, []int{2}) {
		t.Fatalf("timeline %+v", tl)
	}
	// the page keeps the facts of this save and the previous one only
	api.saves.mu.Lock()
	kept := len(api.saves.facts)
	api.saves.mu.Unlock()
	if kept != 2 {
		t.Fatalf("the page keeps %d facts", kept)
	}

	// A new save while a run is busy elsewhere: selecting it does not wait for that run.
	release := make(chan struct{})
	var runFirst []string
	api.saves.runner = func(ctx context.Context, first []string, progress func(string)) error {
		runFirst = first
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	spec.Ticks, spec.ModTime = 240000, now.Add(-time.Hour)
	e.WriteSave("Ennawrin 2.rws", spec)
	e.WriteSave("Other.rws", testkit.SaveSpec{Mods: spec.Mods, ModTime: now.Add(-4 * time.Hour)})
	p = read(2, "")
	sel := p.Colonies[0].Saves[0].Ref
	for tk := savesTask(api); tk == nil || tk.State != app.TaskRunning; tk = savesTask(api) {
		time.Sleep(10 * time.Millisecond)
	}
	if !slices.Equal(runFirst, []string{sel}) {
		t.Fatalf("the run reads %q first, want the selected %s", runFirst, sel)
	}
	start := time.Now()
	d, err = api.SaveDetail(sel)
	if err != nil || d.Play != nil || !d.Reading {
		t.Fatalf("unread detail: play %+v reading %v %v", d.Play, d.Reading, err)
	}
	if el := time.Since(start); el > time.Second {
		t.Fatalf("SaveDetail waited %v", el)
	}
	awaitNews(t, news, "the selected save read", func(ev SavesReadEvent) bool { return slices.Contains(ev.Refs, sel) })
	if d, err = api.SaveDetail(sel); err != nil || d.Play == nil || d.Play.Day != "day 4" || d.Play.Since != "Ennawrin" {
		t.Fatalf("after its read: %+v %v", d.Play, err)
	}
	// the relist gives it its day while the run is still busy
	ev := awaitNews(t, news, "a relist", func(ev SavesReadEvent) bool { return ev.Page != nil })
	if got := rows(ev.Page)[0]; got != "Iron Pact/Ennawrin 2 day 4" {
		t.Fatalf("relisted %q", got)
	}

	// Stopping the task ends the run; what was read stays.
	tk := savesTask(api)
	if err := api.tasks.Stop(tk.ID); err != nil {
		t.Fatal(err)
	}
	end = awaitNews(t, news, "the stop", func(ev SavesReadEvent) bool { return ev.Done })
	if tk = savesTask(api); tk.State != app.TaskStopped || !strings.HasPrefix(end.Text, "stopped reading the saves") {
		t.Fatalf("stopped task %+v, end %+v", tk, end)
	}

	// Leaving the page drops the facts; the window closing ends a run that is going.
	api.leaveSaves()
	api.saves.mu.Lock()
	kept = len(api.saves.facts)
	api.saves.mu.Unlock()
	if kept != 0 {
		t.Fatalf("after leaving, %d facts kept", kept)
	}
	read(3, "")
	for tk = savesTask(api); tk == nil || tk.State != app.TaskRunning; tk = savesTask(api) {
		time.Sleep(10 * time.Millisecond)
	}
	closed := make(chan struct{})
	go func() { api.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("closing the window waits on the run")
	}
	if tk = savesTask(api); tk.State != app.TaskStopped || !tk.Closed {
		t.Fatalf("after closing: %+v", tk)
	}
}

// A save that breaks off before the game's time has no play facts to show: the right card
// says where it breaks off instead of a card of zeros.
func TestSavesBrokenBeforePlay(t *testing.T) {
	e, api, events := fixture(t)
	_, news := savesDrain(events)
	x := testkit.SaveXML(testkit.SaveSpec{Mods: []string{"ludeon.rimworld"}})
	x = x[:strings.Index(x, "<tickManager>")+len("<tickMan")]
	p := filepath.Join(e.SavesDir(), "Broken.rws")
	e.WriteFile(p, x)
	at := e.Now().Add(-time.Hour)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
	api.SavesRead(1, "")
	if end := awaitNews(t, news, "the run's end", func(ev SavesReadEvent) bool { return ev.Done }); end.Error != "" {
		t.Fatalf("end %+v", end)
	}
	d, err := api.SaveDetail("Broken")
	if err != nil || d.Play != nil || d.Reading || !strings.Contains(d.PlayNote, "before its play facts") {
		t.Fatalf("broken save: play %+v reading %v note %q %v", d.Play, d.Reading, d.PlayNote, err)
	}
}
