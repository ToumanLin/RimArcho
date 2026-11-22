// Spike corpusweekly — the weekly log-corpus run's own steps (05 §4.6, 19 §6). The corpus
// repository's scripts/weekly.ps1 runs the collector and the generators; this tool keeps the
// extracted features, picks the counting window and turns the generators' output into the
// bundle Archo Monitor ships.
//
//	corpusweekly features -parsed P -results R -out <features>    keep per-log features by week
//	corpusweekly newids -parsed P -features F -mm 1.6             logs without features yet
//	corpusweekly window -features F -mm 1.6 -week 2026-W47        the files of the last 26 weeks
//	corpusweekly prune -corpus <hugslogs> -features F -keep 8     delete raw logs older than 8 weeks
//	corpusweekly bundle -week W -gen <dir> -prev P -out O -summary S
//
// Features are the collector's parsed records and corpusrun's results, one gzip JSON-lines file
// pair per game version and ISO week of upload: features/<mm>/<week>.{parsed,results}.jsonl.gz.
// They stay in the private corpus repository like the raw logs. The bundle holds aggregates only
// (05 §4.6): no log ids, no mod lists, every entry from at least five players.
package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/corpus"
	"github.com/ToumanLin/RimArcho/spikes/internal/jsonl"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: corpusweekly features|window|prune|bundle [flags]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	parsed := fs.String("parsed", "", "features: hugslogs parsed.jsonl (comma-separated, .gz allowed)")
	results := fs.String("results", "", "features: corpusrun results (comma-separated, .gz allowed)")
	out := fs.String("out", "", "features: features folder; bundle: corpus.json.gz to write")
	features := fs.String("features", "", "window, prune: features folder")
	mm := fs.String("mm", "1.6", "window: game major.minor")
	week := fs.String("week", corpus.Week(time.Now().UTC()), "window, bundle: the ISO week being built")
	weeks := fs.Int("weeks", 26, "window: weeks counted")
	part := fs.String("part", "parsed", "window: parsed or results")
	corpusDir := fs.String("corpus", "", "prune: the hugslogs folder (index.jsonl, raw/)")
	keep := fs.Int("keep", 8, "prune: weeks of raw logs to keep")
	dry := fs.Bool("dry-run", false, "prune: only count")
	gen := fs.String("gen", "", "bundle: generator output, <gen>/<mm>/{patterns,known,observed}.json and logs.txt")
	prev := fs.String("prev", "", "bundle: last week's corpus.json.gz (the summary's baseline)")
	summary := fs.String("summary", "", "bundle: change summary to write (Markdown)")
	steamdb := fs.String("steamdb", "", "bundle: RimSort steamDB.json (packageId → Workshop id) for \"may be fixed\"")
	fresh := fs.Int("fresh", 12, "bundle: drop entries without a supporting log in this many weeks")
	since := fs.String("since", "", "features: only logs uploaded in this ISO week or later")
	fs.Parse(os.Args[2:])
	if *since == "" && (os.Args[1] == "features" || os.Args[1] == "newids") {
		// Only the weeks the next bundle counts.
		if t, err := weekStart(*week); err == nil {
			*since = corpus.Week(t.AddDate(0, 0, -7*(*weeks-1)))
		}
	}
	var err error
	switch os.Args[1] {
	case "features":
		err = keepFeatures(*parsed, *results, *out, *since)
	case "newids":
		err = newIDs(*parsed, *features, *mm, *since)
	case "window":
		var files []string
		files, err = window(*features, *mm, *week, *weeks, *part)
		fmt.Println(strings.Join(files, ","))
	case "prune":
		err = prune(*corpusDir, *features, *keep, *dry)
	case "bundle":
		err = bundle(*week, *gen, *prev, *out, *summary, *steamdb, *fresh)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// ---- features ----

type head struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	MM      string `json:"mm"`
	Created string `json:"created"`
}

// newIDs prints the ids of parsed logs of one game version that have no results features
// yet: the logs corpusrun still has to attribute.
func newIDs(parsedList, features, mm, since string) error {
	have := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(features, mm, "*.results.jsonl.gz"))
	for _, f := range files {
		if err := jsonl.Each(f, func(b []byte) error {
			var h head
			if json.Unmarshal(b, &h) == nil {
				have[h.ID] = true
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return jsonl.Each(parsedList, func(b []byte) error {
		var h head
		if json.Unmarshal(b, &h) != nil || h.Status != "ok" || h.MM != mm || have[h.ID] {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, h.Created); err != nil || since != "" && corpus.Week(t.UTC()) < since {
			return nil
		}
		fmt.Println(h.ID)
		return nil
	})
}

func keepFeatures(parsedList, resultsList, out, since string) error {
	if out == "" {
		return fmt.Errorf("-out is required")
	}
	where := map[string]string{} // id → <mm>/<week>
	adds := map[string]map[string][]byte{}
	add := func(key, part, id string, line []byte) {
		k := key + "." + part
		if adds[k] == nil {
			adds[k] = map[string][]byte{}
		}
		adds[k][id] = append([]byte(nil), bytes.TrimRight(line, "\r\n")...)
	}
	if err := jsonl.Each(parsedList, func(b []byte) error {
		var h head
		if json.Unmarshal(b, &h) != nil || h.Status != "ok" || (h.MM != "1.6" && h.MM != "1.7") {
			return nil
		}
		t, err := time.Parse(time.RFC3339, h.Created)
		if err != nil {
			return nil
		}
		if since != "" && corpus.Week(t.UTC()) < since {
			return nil
		}
		key := h.MM + "/" + corpus.Week(t.UTC())
		where[h.ID] = key
		add(key, "parsed", h.ID, b)
		return nil
	}); err != nil {
		return err
	}
	if resultsList != "" {
		if err := jsonl.Each(resultsList, func(b []byte) error {
			var h head
			if json.Unmarshal(b, &h) != nil {
				return nil
			}
			if key := where[h.ID]; key != "" {
				add(key, "results", h.ID, b)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(adds))
	for k := range adds {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p := filepath.Join(out, filepath.FromSlash(k)+".jsonl.gz")
		n, err := merge(p, adds[k])
		if err != nil {
			return err
		}
		fmt.Printf("%s: +%d\n", k, n)
	}
	return nil
}

// merge adds lines to a gzip JSON-lines file, keeping one line per id (a later run's line
// replaces an earlier one: a rerun of corpusrun with newer attribution).
func merge(path string, lines map[string][]byte) (int, error) {
	have := map[string][]byte{}
	var order []string
	if _, err := os.Stat(path); err == nil {
		if err := jsonl.Each(path, func(b []byte) error {
			var h head
			if json.Unmarshal(b, &h) == nil && h.ID != "" {
				if _, ok := have[h.ID]; !ok {
					order = append(order, h.ID)
				}
				have[h.ID] = append([]byte(nil), bytes.TrimRight(b, "\r\n")...)
			}
			return nil
		}); err != nil {
			return 0, err
		}
	}
	added := 0
	ids := make([]string, 0, len(lines))
	for id := range lines {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, ok := have[id]; !ok {
			order = append(order, id)
			added++
		}
		have[id] = lines[id]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	zw := gzip.NewWriter(f)
	bw := bufio.NewWriterSize(zw, 1<<20)
	for _, id := range order {
		bw.Write(have[id])
		bw.WriteByte('\n')
	}
	err = bw.Flush()
	if e := zw.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return added, os.Rename(tmp, path)
}

// ---- window ----

// window lists the feature files of one game version in the weeks counted, oldest first.
// 1.6 and 1.7 never borrow from each other: each has its own folder.
func window(features, mm, week string, weeks int, part string) ([]string, error) {
	first, err := weekStart(week)
	if err != nil {
		return nil, err
	}
	from := corpus.Week(first.AddDate(0, 0, -7*(weeks-1)))
	ents, err := os.ReadDir(filepath.Join(features, mm))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	suffix := "." + part + ".jsonl.gz"
	var out []string
	for _, e := range ents {
		w, ok := strings.CutSuffix(e.Name(), suffix)
		if !ok || !corpus.ValidWeek(w) || w < from || w > week {
			continue
		}
		out = append(out, filepath.Join(features, mm, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// weekStart is the Monday of an ISO week.
func weekStart(week string) (time.Time, error) {
	if !corpus.ValidWeek(week) {
		return time.Time{}, fmt.Errorf("%q is not an ISO week (2026-W47)", week)
	}
	var y, w int
	fmt.Sscanf(week, "%d-W%d", &y, &w)
	// January 4th is always in week 1.
	t := time.Date(y, 1, 4, 0, 0, 0, 0, time.UTC)
	t = t.AddDate(0, 0, -((int(t.Weekday())+6)%7)+7*(w-1))
	if corpus.Week(t) != week {
		return time.Time{}, fmt.Errorf("%q is not an ISO week", week)
	}
	return t, nil
}

// ---- prune ----

// prune deletes raw logs uploaded more than keep weeks ago whose features are kept. The
// files stay in the corpus repository's history; the working tree keeps its -MaxRawGB cap.
func prune(dir, features string, keep int, dry bool) error {
	if dir == "" || features == "" {
		return fmt.Errorf("-corpus and -features are required")
	}
	kept := map[string]bool{}
	for _, mm := range []string{"1.6", "1.7"} {
		files, _ := filepath.Glob(filepath.Join(features, mm, "*.parsed.jsonl.gz"))
		for _, f := range files {
			if err := jsonl.Each(f, func(b []byte) error {
				var h head
				if json.Unmarshal(b, &h) == nil {
					kept[h.ID] = true
				}
				return nil
			}); err != nil {
				return err
			}
		}
	}
	cut := time.Now().UTC().AddDate(0, 0, -7*keep)
	n, skipped := 0, 0
	var bytes int64
	err := jsonl.Each(filepath.Join(dir, "index.jsonl"), func(b []byte) error {
		var e struct{ ID, Created string }
		if json.Unmarshal(b, &e) != nil {
			return nil
		}
		t, err := time.Parse(time.RFC3339, e.Created)
		if err != nil || !t.Before(cut) {
			return nil
		}
		p := filepath.Join(dir, "raw", e.ID+".txt")
		fi, err := os.Stat(p)
		if err != nil {
			return nil
		}
		if !kept[e.ID] {
			skipped++ // no features yet: keep the raw log until they are extracted
			return nil
		}
		n++
		bytes += fi.Size()
		if !dry {
			return os.Remove(p)
		}
		return nil
	})
	verb := "deleted"
	if dry {
		verb = "would delete"
	}
	fmt.Printf("raw logs older than %d weeks: %s %d (%.2f GB); kept %d without features\n", keep, verb, n, float64(bytes)/(1<<30), skipped)
	return err
}

// ---- bundle ----

func bundle(week, gen, prevPath, out, summaryPath, steamdb string, freshWeeks int) error {
	if !corpus.ValidWeek(week) || gen == "" || out == "" {
		return fmt.Errorf("-week, -gen and -out are required")
	}
	start, err := weekStart(week)
	if err != nil {
		return err
	}
	fresh := start.AddDate(0, 0, -7*(freshWeeks-1)).Format("2006-01-02")
	ws := workshopTimes(steamdb)
	b := &corpus.Bundle{Format: corpus.Format, Data: week, Generated: time.Now().UTC().Format("2006-01-02"), Sections: map[string]*corpus.Section{}}
	stats := map[string]*sectionStats{}
	for _, mm := range []string{"1.6", "1.7"} {
		dir := filepath.Join(gen, mm)
		pat, err := os.ReadFile(filepath.Join(dir, "patterns.json"))
		if err != nil {
			continue // no logs of this game version yet
		}
		st := &sectionStats{}
		stats[mm] = st
		s := &corpus.Section{Data: week, Window: readText(filepath.Join(dir, "window.txt"))}
		if s.Patterns, s.Logs, err = filterPatterns(pat, fresh, ws, st); err != nil {
			return fmt.Errorf("%s patterns: %w", mm, err)
		}
		if k, err := os.ReadFile(filepath.Join(dir, "known.json")); err == nil {
			if s.Known, err = filterKnown(k, fresh, ws, st); err != nil {
				return fmt.Errorf("%s known: %w", mm, err)
			}
		}
		if o, err := os.ReadFile(filepath.Join(dir, "observed.json")); err == nil {
			if s.Observed, err = filterObserved(o, fresh, st); err != nil {
				return fmt.Errorf("%s observed: %w", mm, err)
			}
		}
		b.Sections[mm] = s
	}
	if len(b.Sections) == 0 {
		return fmt.Errorf("%s has no generator output", gen)
	}
	enc, err := corpus.Encode(b)
	if err != nil {
		return err
	}
	if _, err := corpus.Parse(enc); err != nil {
		return fmt.Errorf("the new bundle does not read back: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, enc, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %s, %d bytes\n", out, week, len(enc))
	if summaryPath != "" {
		var prev *corpus.Bundle
		if raw, err := os.ReadFile(prevPath); err == nil {
			prev, _ = corpus.Parse(raw)
		}
		return os.WriteFile(summaryPath, []byte(summarize(b, prev, stats, freshWeeks)), 0o644)
	}
	return nil
}

func readText(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

type sectionStats struct {
	droppedStale, mayBeFixed int
}

// filterPatterns drops mod-specific patterns without a supporting log since fresh and marks
// those whose Mod was updated on the Workshop after their last log. Generic rules stay.
func filterPatterns(raw []byte, fresh string, ws map[string]int64, st *sectionStats) (json.RawMessage, int, error) {
	var d map[string]json.RawMessage
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, 0, err
	}
	var logs int
	json.Unmarshal(d["logs"], &logs)
	var mods []map[string]any
	json.Unmarshal(d["mod"], &mods)
	var keep []map[string]any
	for _, p := range mods {
		last, _ := p["lastLog"].(string)
		if last != "" && last < fresh {
			st.droppedStale++
			continue
		}
		if mod, _ := p["mod"].(string); updatedAfter(ws, mod, last) {
			p["mayBeFixed"] = true
			st.mayBeFixed++
		}
		keep = append(keep, p)
	}
	mb, _ := json.Marshal(keep)
	d["mod"] = mb
	out, err := json.Marshal(d)
	return out, logs, err
}

func filterKnown(raw []byte, fresh string, ws map[string]int64, st *sectionStats) (json.RawMessage, error) {
	var d map[string]json.RawMessage
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	for _, key := range []string{"prereqs", "combos"} {
		var es []map[string]any
		json.Unmarshal(d[key], &es)
		var keep []map[string]any
		for _, e := range es {
			last, _ := e["lastLog"].(string)
			if last != "" && last < fresh {
				st.droppedStale++
				continue
			}
			mod, _ := e["mod"].(string)
			if ms, ok := e["mods"].([]any); ok && len(ms) > 0 {
				mod, _ = ms[0].(string)
			}
			if updatedAfter(ws, mod, last) {
				e["mayBeFixed"] = true
				st.mayBeFixed++
			}
			keep = append(keep, e)
		}
		if keep == nil {
			keep = []map[string]any{}
		}
		d[key], _ = json.Marshal(keep)
	}
	return json.Marshal(d)
}

func filterObserved(raw []byte, fresh string, st *sectionStats) (json.RawMessage, error) {
	var d map[string]json.RawMessage
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	for _, key := range []string{"harmless", "harmful"} {
		var es []map[string]any
		json.Unmarshal(d[key], &es)
		var keep []map[string]any
		for _, e := range es {
			if last, _ := e["lastLog"].(string); last != "" && last < fresh {
				st.droppedStale++
				continue
			}
			keep = append(keep, e)
		}
		if keep == nil {
			keep = []map[string]any{}
		}
		d[key], _ = json.Marshal(keep)
	}
	return json.Marshal(d)
}

// updatedAfter: the Mod's Workshop item changed after the day of its last supporting log.
func updatedAfter(ws map[string]int64, pkg, last string) bool {
	t, ok := ws[strings.ToLower(pkg)]
	if !ok || last == "" {
		return false
	}
	day, err := time.Parse("2006-01-02", last)
	if err != nil {
		return false
	}
	return time.Unix(t, 0).After(day.AddDate(0, 0, 1))
}

// workshopTimes reads time_updated of the Workshop items steamDB maps packageIds to, through
// the anonymous Steam Web API (100 items per request). Without -steamdb, nothing is marked.
// The bundle step runs it for every packageId in steamDB once; failures leave items unmarked.
func workshopTimes(steamdb string) map[string]int64 {
	out := map[string]int64{}
	if steamdb == "" {
		return out
	}
	raw, err := os.ReadFile(steamdb)
	if err != nil {
		fmt.Fprintln(os.Stderr, "steamdb:", err)
		return out
	}
	var db struct {
		Database map[string]struct {
			PackageID string `json:"packageId"`
		} `json:"database"`
	}
	if json.Unmarshal(raw, &db) != nil {
		return out
	}
	pkgOf := map[string]string{}
	var ids []string
	for id, e := range db.Database {
		if e.PackageID == "" || strings.Trim(id, "0123456789") != "" {
			continue
		}
		pkgOf[id] = strings.ToLower(e.PackageID)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	client := &http.Client{Timeout: 30 * time.Second}
	for i := 0; i < len(ids); i += 100 {
		batch := ids[i:min(i+100, len(ids))]
		form := url.Values{"itemcount": {fmt.Sprint(len(batch))}}
		for j, id := range batch {
			form.Set(fmt.Sprintf("publishedfileids[%d]", j), id)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "POST", "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		cancel()
		var r struct {
			Response struct {
				Details []struct {
					ID      string `json:"publishedfileid"`
					Updated int64  `json:"time_updated"`
				} `json:"publishedfiledetails"`
			} `json:"response"`
		}
		if json.Unmarshal(body, &r) != nil {
			continue
		}
		for _, d := range r.Response.Details {
			if p := pkgOf[d.ID]; p != "" && d.Updated > out[p] {
				out[p] = d.Updated
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Printf("Workshop update times: %d packages\n", len(out))
	return out
}

// ---- summary ----

type entryKey struct{ kind, key string }

type entry struct {
	logs int
	text string
}

func entries(s *corpus.Section) map[entryKey]entry {
	out := map[entryKey]entry{}
	if s == nil {
		return out
	}
	var p struct {
		Mod []struct {
			ID       string `json:"id"`
			Mod      string `json:"mod"`
			Logs     int    `json:"logs"`
			Template string `json:"template"`
		} `json:"mod"`
	}
	json.Unmarshal(s.Patterns, &p)
	for _, m := range p.Mod {
		out[entryKey{"pattern", m.ID}] = entry{m.Logs, m.Mod + ": " + short(m.Template)}
	}
	var k struct {
		Prereqs []struct {
			Mod, Needs string
			Logs       int
		} `json:"prereqs"`
		Combos []struct {
			Mods      []string
			Version   string
			Logs      int
			Templates []string
		} `json:"combos"`
	}
	json.Unmarshal(s.Known, &k)
	for _, x := range k.Prereqs {
		out[entryKey{"prereq", x.Mod + ">" + x.Needs}] = entry{x.Logs, x.Mod + " needs " + x.Needs}
	}
	for _, x := range k.Combos {
		key := strings.Join(x.Mods, "+") + "@" + x.Version
		if len(x.Templates) > 0 {
			key += "|" + x.Templates[0]
		}
		out[entryKey{"combo", key}] = entry{x.Logs, strings.Join(x.Mods, " + ") + " " + x.Version}
	}
	var o struct {
		Harmless, Harmful []struct {
			Kind, From, To string
			Violated       int
		}
	}
	json.Unmarshal(s.Observed, &o)
	for _, x := range o.Harmless {
		out[entryKey{"order-harmless", x.Kind + " " + x.From + " " + x.To}] = entry{x.Violated, x.Kind + " " + x.From + " → " + x.To}
	}
	for _, x := range o.Harmful {
		out[entryKey{"order-harmful", x.Kind + " " + x.From + " " + x.To}] = entry{x.Violated, x.Kind + " " + x.From + " → " + x.To}
	}
	return out
}

func short(s string) string {
	if r := []rune(s); len(r) > 90 {
		return string(r[:90]) + "…"
	}
	return s
}

// summarize is the weekly change summary for the maintainer: per game version the entries
// added, removed and those whose log count changed by half or more (and at least 10).
func summarize(b, prev *corpus.Bundle, stats map[string]*sectionStats, fresh int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Corpus %s\n\nformat %d, generated %s", b.Data, b.Format, b.Generated)
	if prev != nil {
		fmt.Fprintf(&sb, "; compared with %s", prev.Data)
	}
	sb.WriteString("\n")
	games := make([]string, 0, len(b.Sections))
	for g := range b.Sections {
		games = append(games, g)
	}
	sort.Strings(games)
	for _, g := range games {
		s := b.Sections[g]
		var ps *corpus.Section
		if prev != nil {
			ps = prev.Sections[g]
		}
		cur, old := entries(s), entries(ps)
		fmt.Fprintf(&sb, "\n## %s\n\n%d logs, window %s; dropped without a log in %d weeks: %d; may be fixed: %d\n",
			g, s.Logs, s.Window, fresh, stats[g].droppedStale, stats[g].mayBeFixed)
		if ps != nil {
			fmt.Fprintf(&sb, "previous: %d logs (%s)\n", ps.Logs, ps.Data)
		}
		var added, removed, big []string
		for k, e := range cur {
			o, ok := old[k]
			switch {
			case !ok:
				added = append(added, fmt.Sprintf("- %s: %s (%d logs)", k.kind, e.text, e.logs))
			case abs(e.logs-o.logs) >= 10 && 2*abs(e.logs-o.logs) >= o.logs:
				big = append(big, fmt.Sprintf("- %s: %s: %d → %d logs", k.kind, e.text, o.logs, e.logs))
			}
		}
		for k, o := range old {
			if _, ok := cur[k]; !ok {
				removed = append(removed, fmt.Sprintf("- %s: %s (%d logs)", k.kind, o.text, o.logs))
			}
		}
		for _, l := range []struct {
			name string
			rows []string
		}{{"Added", added}, {"Removed", removed}, {"Big changes", big}} {
			sort.Strings(l.rows)
			fmt.Fprintf(&sb, "\n### %s (%d)\n\n", l.name, len(l.rows))
			for i, r := range l.rows {
				if i == 200 {
					fmt.Fprintf(&sb, "- … %d more\n", len(l.rows)-200)
					break
				}
				sb.WriteString(r + "\n")
			}
		}
	}
	return sb.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
