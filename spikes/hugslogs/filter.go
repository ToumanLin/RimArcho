package main

// Cleaning filter: every downloaded log gets a verdict right after download, before anything is
// written to raw/. Only kept logs are stored; every candidate (kept or not) is recorded in
// index.jsonl with its verdict, so later runs skip it and filter rates can be reported.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Verdicts. "kept" or a drop reason; parse's own drop reasons (no-log-section, empty-log,
// no-mod-list, log-parse-error, no-game-version) pass through unchanged.
const (
	verdictKept     = "kept"
	dropGone        = "gone"                 // raw URL 404: the gist was deleted
	dropDuplicate   = "duplicate-content"    // same bytes as a log already kept
	dropVersion     = "version-out-of-scope" // not 1.6 or 1.7
	dropNoProblems  = "no-problems"          // 1.6 without a single problem cluster
	dropTooShort    = "too-short"            // fewer than minLogLines log lines: upload cut off or junk
	dropTooLarge    = "too-large"            // over maxRawBytes: GitHub refuses files over 100 MB
	maxRawBytes     = 90 << 20
	minLogLines     = 20
	notProblems    = "other,mod-report" // cluster classes that are not problems
)

// isProblem reports whether a cluster class counts as a problem line (error, exception, xref,
// patch, texture …). "other" (status lines that mention "failed", "missing" …) and mod
// self-reports do not.
func isProblem(class string) bool {
	return class != "" && !strings.Contains(","+notProblems+",", ","+class+",")
}

// contentHash is the exact-duplicate key: the first 8 bytes of the SHA-256 of the raw upload
// (the same value parse records as Record.Hash).
func contentHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// judge decides whether a downloaded log is kept. seen holds the content hashes of kept logs;
// the caller adds the hash when the log is kept.
func judge(id, created string, raw []byte, seen map[string]bool) (verdict, mm, hash string) {
	hash = contentHash(raw)
	if len(raw) > maxRawBytes {
		return dropTooLarge, "", hash
	}
	if seen[hash] {
		return dropDuplicate, "", hash
	}
	r := parse(id, created, raw)
	if r.Status != "ok" {
		return r.Drop, r.MM, hash
	}
	switch r.MM {
	case "1.7":
		return verdictKept, r.MM, hash // every 1.7 log is kept, for a later 1.7 study
	case "1.6":
	default:
		return dropVersion, r.MM, hash
	}
	if r.LogLines < minLogLines {
		return dropTooShort, r.MM, hash
	}
	for _, c := range r.Clusters {
		if isProblem(c.Class) {
			return verdictKept, r.MM, hash
		}
	}
	return dropNoProblems, r.MM, hash
}

// ---- index with verdicts ----

// index is index.jsonl in file order. Lines are rewritten in place (atomically) when verdicts
// change; a gist listed twice keeps its first line.
type index struct {
	path  string
	order []string
	m     map[string]indexEntry
}

func loadIndex(dir string) (*index, error) {
	ix := &index{path: filepath.Join(dir, "index.jsonl"), m: map[string]indexEntry{}}
	f, err := os.Open(ix.path)
	if os.IsNotExist(err) {
		return ix, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e indexEntry
		if json.Unmarshal(sc.Bytes(), &e) != nil || e.ID == "" {
			continue
		}
		old, ok := ix.m[e.ID]
		if !ok {
			ix.order = append(ix.order, e.ID)
			ix.m[e.ID] = e
			continue
		}
		// A later line for the same gist (older collector versions appended): keep the verdict.
		if old.Verdict == "" && e.Verdict != "" {
			old.Verdict, old.MM, old.Hash, old.Fetched = e.Verdict, e.MM, e.Hash, e.Fetched
			ix.m[e.ID] = old
		}
	}
	return ix, sc.Err()
}

// add appends a newly listed gist; it reports false for a known one.
func (ix *index) add(e indexEntry) bool {
	if _, ok := ix.m[e.ID]; ok {
		return false
	}
	ix.order = append(ix.order, e.ID)
	ix.m[e.ID] = e
	return true
}

// save rewrites index.jsonl through a temporary file.
func (ix *index) save() error {
	tmp := ix.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for _, id := range ix.order {
		j, _ := json.Marshal(ix.m[id])
		w.Write(append(j, '\n'))
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, ix.path)
}

// decided reports whether a gist needs no download: it has a verdict, or its raw log is on disk
// (collected before the filter existed).
func decided(dir string, e indexEntry) bool {
	if e.Verdict != "" {
		return true
	}
	_, err := os.Stat(rawPath(dir, e.ID))
	return err == nil
}

// keptHashes returns the content hashes of kept logs, computing (and recording) a verdict for
// raw logs that predate the filter. Those stay on disk whatever their verdict: they are already
// in the repository's history, so deleting them saves nothing.
func keptHashes(dir string, ix *index) (map[string]bool, error) {
	seen := map[string]bool{}
	tagged := 0
	for _, id := range ix.order {
		e := ix.m[id]
		if e.Verdict == verdictKept && e.Hash != "" {
			seen[e.Hash] = true
		}
	}
	// Created order, so the first upload of identical content is the kept one.
	ids := append([]string(nil), ix.order...)
	sort.SliceStable(ids, func(a, b int) bool { return ix.m[ids[a]].Created < ix.m[ids[b]].Created })
	for _, id := range ids {
		e := ix.m[id]
		if e.Verdict != "" {
			continue
		}
		b, err := os.ReadFile(rawPath(dir, id))
		if err != nil {
			continue
		}
		e.Verdict, e.MM, e.Hash = judge(id, e.Created, b, seen)
		if e.Verdict == verdictKept {
			seen[e.Hash] = true
		}
		e.Legacy = true
		ix.m[id] = e
		tagged++
	}
	if tagged > 0 {
		fmt.Printf("tagged %d raw logs collected before the filter\n", tagged)
		if err := ix.save(); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

// report prints verdict counts for the index entries that keep accepts.
func report(ix *index, keep func(indexEntry) bool) {
	verdicts := map[string]int{}
	mm := map[string]int{}
	n := 0
	for _, id := range ix.order {
		e := ix.m[id]
		if e.Verdict == "" || !keep(e) {
			continue
		}
		n++
		verdicts[e.Verdict]++
		if e.Verdict == verdictKept {
			mm[e.MM]++
		}
	}
	fmt.Printf("judged %d: %s\n", n, fmtCounts(verdicts, 0))
	if n > 0 {
		fmt.Printf("  kept %d (%.1f%%), by version: %s\n", verdicts[verdictKept], pct(verdicts[verdictKept], n), fmtCounts(mm, 0))
	}
}

// ---- even sampling ----

// dayOrder is the order in which days of a month are sampled: the 15th first (the old sample
// mode), then spread over the month so that a partial month is still spread out.
var dayOrder = []int{15, 1, 8, 22, 4, 11, 18, 25, 28, 6, 13, 20, 27, 2, 9, 16, 23, 5, 12, 19, 26, 3, 10, 17, 24, 7, 14, 21, 29, 30, 31}

// even downloads whole days of each month, in dayOrder, until the month has perMonth kept logs
// (counting logs kept earlier), for months at or after from. It stops after maxKept newly kept
// logs (0 = no limit) so the caller can commit and push in batches; a rerun resumes.
func even(dir, from, until string, perMonth, workers, maxKept int) error {
	ix, err := loadIndex(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		return err
	}
	seen, err := keptHashes(dir, ix)
	if err != nil {
		return err
	}
	inWindow := func(e indexEntry) bool {
		return (from == "" || e.Created >= from) && (until == "" || e.Created < until)
	}
	// month -> day -> undecided entries; month -> kept so far
	todo := map[string]map[int][]indexEntry{}
	kept := map[string]int{}
	for _, id := range ix.order {
		e := ix.m[id]
		if !inWindow(e) || len(e.Created) < 10 {
			continue
		}
		t, err := time.Parse(time.RFC3339, e.Created)
		if err != nil {
			continue
		}
		mon := t.UTC().Format("2006-01")
		if todo[mon] == nil {
			todo[mon] = map[int][]indexEntry{}
		}
		if e.Verdict == verdictKept {
			kept[mon]++
		}
		if !decided(dir, e) {
			todo[mon][t.UTC().Day()] = append(todo[mon][t.UTC().Day()], e)
		}
	}
	months := make([]string, 0, len(todo))
	for m := range todo {
		months = append(months, m)
	}
	sort.Strings(months)
	fmt.Printf("%d months, quota %d kept per month\n", len(months), perMonth)
	next := map[string]int{} // position in dayOrder
	newKept := 0
	for round := 0; ; round++ {
		// One day from every month under quota per round, downloaded together.
		var batch []indexEntry
		for _, m := range months {
			if kept[m] >= perMonth {
				continue
			}
			for next[m] < len(dayOrder) {
				d := dayOrder[next[m]]
				es := todo[m][d]
				if len(es) == 0 {
					next[m]++
					continue
				}
				// Not more than the month still needs (at the observed keep rate this
				// over-fetches a little); the rest of the day is taken next round.
				need := perMonth - kept[m]
				if len(es) > need*2 {
					batch = append(batch, es[:need*2]...)
					todo[m][d] = es[need*2:]
				} else {
					batch = append(batch, es...)
					todo[m][d] = nil
					next[m]++
				}
				break
			}
		}
		if len(batch) == 0 {
			break
		}
		limit := 0
		if maxKept > 0 {
			limit = maxKept - newKept
		}
		res, err := fetchJudge(dir, ix, seen, batch, workers, limit)
		if err != nil {
			return err
		}
		for m, n := range res {
			kept[m] += n
			newKept += n
		}
		var line []string
		for _, m := range months {
			line = append(line, fmt.Sprintf("%s %d", m[2:], kept[m]))
		}
		fmt.Printf("round %d: +%d kept this run; kept per month: %s\n", round+1, newKept, strings.Join(line, ", "))
		if maxKept > 0 && newKept >= maxKept {
			fmt.Printf("stopped at -max-kept %d; run again to continue\n", maxKept)
			break
		}
	}
	report(ix, inWindow)
	short := 0
	for _, m := range months {
		if kept[m] < perMonth {
			short++
		}
	}
	if !(maxKept > 0 && newKept >= maxKept) {
		fmt.Printf("done: %d months reach the quota, %d have fewer logs than the quota\n", len(months)-short, short)
	}
	fmt.Printf("new kept this run: %d\n", newKept)
	return nil
}

// fetchJudge downloads the entries, judges each, stores kept logs in raw/ and records every
// verdict in the index. It stops dispatching once limit logs are kept (0 = no limit). It
// returns kept logs per month.
func fetchJudge(dir string, ix *index, seen map[string]bool, todo []indexEntry, workers, limit int) (map[string]int, error) {
	ch := make(chan indexEntry)
	var mu sync.Mutex
	perMonth := map[string]int{}
	keptN, done, failed := 0, 0, 0
	stop := false
	var wg sync.WaitGroup
	for w := 0; w < max(workers, 1); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range ch {
				mu.Lock()
				s := stop
				mu.Unlock()
				if s {
					continue // the limit was reached while this one waited
				}
				b, code, err := get(fmt.Sprintf("%s/%s/%s/raw", rawBase, account, e.ID))
				now := time.Now().UTC().Format(time.RFC3339)
				mu.Lock()
				switch {
				case err == nil && code == 404:
					e.Verdict, e.Fetched = dropGone, now
				case err != nil || code != 200:
					failed++ // no verdict: retried next run
					fmt.Fprintf(os.Stderr, "raw %s: HTTP %d %v\n", e.ID, code, err)
				default:
					e.Verdict, e.MM, e.Hash = judge(e.ID, e.Created, b, seen)
					e.Fetched = now
					if e.Verdict == verdictKept && limit > 0 && keptN >= limit {
						// Another worker reached the limit first: leave it for the next run.
						e.Verdict, e.MM, e.Hash, e.Fetched = "", "", "", ""
					} else if e.Verdict == verdictKept {
						if werr := os.WriteFile(rawPath(dir, e.ID), b, 0o644); werr != nil {
							mu.Unlock()
							panic(werr)
						}
						seen[e.Hash] = true
						keptN++
						perMonth[e.Created[:7]]++
						if limit > 0 && keptN >= limit {
							stop = true
						}
					}
				}
				ix.m[e.ID] = e
				done++
				if done%500 == 0 {
					fmt.Printf("  %d / %d judged, %d kept, %d failed\n", done, len(todo), keptN, failed)
					if err := ix.save(); err != nil {
						fmt.Fprintln(os.Stderr, "saving index:", err)
					}
				}
				mu.Unlock()
				time.Sleep(rawDelay)
			}
		}()
	}
	for _, e := range todo {
		mu.Lock()
		s := stop
		mu.Unlock()
		if s {
			break
		}
		ch <- e
	}
	close(ch)
	wg.Wait()
	if failed > 0 {
		fmt.Printf("  %d downloads failed (no verdict; retried next run)\n", failed)
	}
	return perMonth, ix.save()
}

// verdicts prints filter rates and the version share for gists created in [from, until).
func verdicts(dir, from, until string) error {
	ix, err := loadIndex(dir)
	if err != nil {
		return err
	}
	f, u := from, until
	if f != "" {
		if f, err = resolveFrom(dir, from); err != nil {
			return err
		}
	}
	if u != "" {
		if u, err = resolveFrom(dir, until); err != nil {
			return err
		}
	}
	inWindow := func(e indexEntry) bool {
		return (f == "" || e.Created >= f) && (u == "" || e.Created < u)
	}
	listed := 0
	for _, id := range ix.order {
		if inWindow(ix.m[id]) {
			listed++
		}
	}
	fmt.Printf("listed %d gists", listed)
	if from != "" || until != "" {
		fmt.Printf(" created in [%s, %s)", from, until)
	}
	fmt.Println()
	report(ix, inWindow)
	report(ix, func(e indexEntry) bool { return inWindow(e) && !e.Legacy })
	fmt.Println("  (second line: judged by the filter at download time, without logs collected before it)")
	return nil
}
