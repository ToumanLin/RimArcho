// Spike P0 of the log corpus research (docs/research/00-日志语料顶层设计.md §10):
// can HugsLib logs be collected in bulk, what do their sections look like, how many survive
// cleaning, and are there enough repeat uploads (sessions) to mine fix evidence from.
//
//	go run ./spikes/hugslogs fetch -dir <corpus>   # web listing (newest ~1,000 only), download raw logs
//	go run ./spikes/hugslogs probe                 # GitHub API: how far back the listing goes
//	go run ./spikes/hugslogs api -dir <corpus> -from 2025-07-01 [-max N]   # GitHub API listing + download
//	go run ./spikes/hugslogs stats -dir <corpus>   # parse, clean, print RESULT numbers
//
// probe and api need a GitHub token (GITHUB_TOKEN, GH_TOKEN or `gh auth login`). They do not work
// in the cloud session, whose proxy only allows repository-scoped API paths.
//
// The corpus directory holds raw logs and parsed records. It lives in the private
// ToumanLin/RimArcho-corpus repository, never in this one.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

const (
	account = "HugsLibRecordKeeper"
	ua      = "RimArcho-research-spike (contact via github.com/ToumanLin/RimArcho)"
)

// Endpoints; tests point them at a local server.
var (
	apiBase = "https://api.github.com"
	rawBase = "https://gist.githubusercontent.com"
	// rawDelay spaces raw downloads per worker.
	rawDelay = 500 * time.Millisecond
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: hugslogs fetch|probe|api|stats [flags]   (hugslogs <command> -h for flags)")
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	dir := fs.String("dir", "/home/user/rimarcho-corpus/hugslogs", "corpus directory (the hugslogs folder of RimArcho-corpus)")
	pages := fs.Int("pages", 0, "listing pages to read; fetch: 10 gists per page, default 100; api: 100 gists per page, default no limit")
	workers := fs.Int("workers", 2, "parallel raw downloads")
	from := fs.String("from", "", "api: only gists created at or after this time (RFC 3339 or YYYY-MM-DD); default: 3 days before the newest indexed gist")
	maxDL := fs.Int("max", 0, "api: download at most this many raw logs this run, newest first (0 = no limit)")
	fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "fetch":
		p := *pages
		if p == 0 {
			p = 100
		}
		err = fetch(*dir, p, *workers)
	case "probe":
		err = probe()
	case "api":
		f, ferr := resolveFrom(*dir, *from)
		if ferr != nil {
			err = ferr
			break
		}
		fmt.Printf("listing gists created since %s\n", f)
		err = apiFetch(*dir, f, *pages, *workers, *maxDL)
	case "stats":
		err = stats(*dir)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// resolveFrom turns -from into an RFC 3339 time. Without -from it is 3 days before the newest
// indexed gist (an incremental run), or 3 days ago for an empty index.
func resolveFrom(dir, from string) (string, error) {
	if from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			return t.UTC().Format(time.RFC3339), nil
		}
		return "", fmt.Errorf("-from %q: use YYYY-MM-DD or RFC 3339", from)
	}
	idx, err := readIndex(filepath.Join(dir, "index.jsonl"))
	if err != nil {
		return "", err
	}
	newest := time.Now().UTC()
	if len(idx) > 0 {
		var n string
		for _, e := range idx {
			if e.Created > n {
				n = e.Created
			}
		}
		if t, err := time.Parse(time.RFC3339, n); err == nil {
			newest = t
		}
	}
	return newest.Add(-72 * time.Hour).Format(time.RFC3339), nil
}

// ---- fetch ----

type indexEntry struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Created string `json:"created"`
	Listed  string `json:"listed"` // when this run saw it
}

var listRe = regexp.MustCompile(`href="/` + account + `/([0-9a-f]{20,})"><strong[^>]*>([^<]*)</strong></a>[\s\S]*?datetime="([^"]+)"`)

var client = &http.Client{Timeout: 60 * time.Second}

func get(url string) ([]byte, int, error) {
	var lastErr error
	wait := 5 * time.Second
	for try := 0; try < 6; try++ {
		if try > 0 {
			time.Sleep(wait)
			wait = min(wait*2, 3*time.Minute)
		}
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", ua)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				wait = time.Duration(s+1) * time.Second
			}
			fmt.Fprintf(os.Stderr, "%v, retrying in %v\n", lastErr, wait)
			continue
		}
		return b, resp.StatusCode, nil
	}
	return nil, 0, lastErr
}

func readIndex(path string) (map[string]indexEntry, error) {
	idx := map[string]indexEntry{}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return idx, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e indexEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			idx[e.ID] = e
		}
	}
	return idx, sc.Err()
}

func fetch(dir string, pages, workers int) error {
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		return err
	}
	idxPath := filepath.Join(dir, "index.jsonl")
	idx, err := readIndex(idxPath)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(idxPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	listed, added := 0, 0
	var oldest, newest string
	for p := 1; p <= pages; p++ {
		b, code, err := get(fmt.Sprintf("https://gist.github.com/%s?page=%d", account, p))
		if err != nil {
			// Keep what was listed; the next run continues (the index is append-only).
			fmt.Fprintf(os.Stderr, "listing stopped at page %d: %v\n", p, err)
			break
		}
		ms := listRe.FindAllSubmatch(b, -1)
		if code != 200 || len(ms) == 0 {
			fmt.Printf("listing ends at page %d (HTTP %d)\n", p, code)
			break
		}
		for _, m := range ms {
			e := indexEntry{ID: string(m[1]), File: string(m[2]), Created: string(m[3]), Listed: now}
			listed++
			if newest == "" || e.Created > newest {
				newest = e.Created
			}
			if oldest == "" || e.Created < oldest {
				oldest = e.Created
			}
			if _, ok := idx[e.ID]; ok {
				continue
			}
			idx[e.ID] = e
			added++
			j, _ := json.Marshal(e)
			out.Write(append(j, '\n'))
		}
		time.Sleep(2 * time.Second)
	}
	fmt.Printf("listed %d gists (%s .. %s), %d new, index holds %d\n", listed, oldest, newest, added, len(idx))
	return download(dir, idx, workers, "", 0)
}

// download fetches the raw logs of index entries that are not on disk yet, newest first: only
// entries created at or after from ("" = all), at most max of them (0 = no limit).
func download(dir string, idx map[string]indexEntry, workers int, from string, limit int) error {
	var todo []indexEntry
	for _, e := range idx {
		if from != "" && e.Created < from {
			continue
		}
		if _, err := os.Stat(rawPath(dir, e.ID)); os.IsNotExist(err) {
			todo = append(todo, e)
		}
	}
	sort.Slice(todo, func(a, b int) bool { return todo[a].Created > todo[b].Created })
	if limit > 0 && len(todo) > limit {
		fmt.Printf("%d raw logs to download, capped at -max %d (newest first)\n", len(todo), limit)
		todo = todo[:limit]
	}
	ch := make(chan indexEntry)
	var mu sync.Mutex
	ok, failed := 0, 0
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range ch {
				b, code, err := get(fmt.Sprintf("%s/%s/%s/raw", rawBase, account, e.ID))
				mu.Lock()
				if err != nil || code != 200 {
					failed++
					fmt.Fprintf(os.Stderr, "raw %s: HTTP %d %v\n", e.ID, code, err)
				} else if werr := os.WriteFile(rawPath(dir, e.ID), b, 0o644); werr != nil {
					failed++
				} else {
					ok++
				}
				if n := ok + failed; n%200 == 0 {
					fmt.Printf("  %d / %d downloaded (%d failed)\n", n, len(todo), failed)
				}
				mu.Unlock()
				time.Sleep(rawDelay)
			}
		}()
	}
	for _, e := range todo {
		ch <- e
	}
	close(ch)
	wg.Wait()
	fmt.Printf("downloaded %d raw logs, %d failed\n", ok, failed)
	return nil
}

func rawPath(dir, id string) string { return filepath.Join(dir, "raw", id+".txt") }

// ---- GitHub API (for running outside the cloud session, where the API is not blocked) ----

// token returns a GitHub token from GITHUB_TOKEN, GH_TOKEN or `gh auth token`.
func token() (string, error) {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if t := strings.TrimSpace(os.Getenv(k)); t != "" {
			return t, nil
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if t := strings.TrimSpace(string(out)); err == nil && t != "" {
		return t, nil
	}
	return "", fmt.Errorf("no GitHub token: set GITHUB_TOKEN, or log in with `gh auth login`")
}

type apiGist struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
	Files     map[string]struct {
		Filename string `json:"filename"`
		Size     int    `json:"size"`
	} `json:"files"`
}

// apiPage lists one page (100 gists) of the account's gists. It waits out rate limits.
func apiPage(tok string, page int) ([]apiGist, error) {
	url := fmt.Sprintf("%s/users/%s/gists?per_page=100&page=%d", apiBase, account, page)
	for try := 0; try < 6; try++ {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		switch {
		case resp.StatusCode == 200:
			var gs []apiGist
			if err := json.Unmarshal(b, &gs); err != nil {
				return nil, fmt.Errorf("page %d: %v", page, err)
			}
			return gs, nil
		case resp.StatusCode == 429 || resp.StatusCode == 403 && (resp.Header.Get("Retry-After") != "" ||
			resp.Header.Get("X-RateLimit-Remaining") == "0" || strings.Contains(strings.ToLower(string(b)), "rate limit")):
			wait := 60 * time.Second
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				wait = time.Duration(s+1) * time.Second
			} else if resp.Header.Get("X-RateLimit-Remaining") == "0" {
				if r, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
					wait = time.Until(time.Unix(r, 0)) + 5*time.Second
				}
			}
			fmt.Fprintf(os.Stderr, "page %d: HTTP %d (rate limit), waiting %v\n", page, resp.StatusCode, wait.Round(time.Second))
			time.Sleep(wait)
		case resp.StatusCode >= 500:
			time.Sleep(10 * time.Second)
		default:
			return nil, fmt.Errorf("page %d: HTTP %d: %s", page, resp.StatusCode, trunc(string(b), 300))
		}
	}
	return nil, fmt.Errorf("page %d: giving up after retries", page)
}

// probe answers "how deep does the API listing go": a few pages far apart, then a binary
// search for the last non-empty page. It downloads nothing.
func probe() error {
	tok, err := token()
	if err != nil {
		return err
	}
	show := func(p int) (int, string) {
		gs, err := apiPage(tok, p)
		if err != nil {
			fmt.Printf("page %5d: error %v\n", p, err)
			return -1, ""
		}
		if len(gs) == 0 {
			fmt.Printf("page %5d: empty\n", p)
			return 0, ""
		}
		fmt.Printf("page %5d: %3d gists, %s .. %s\n", p, len(gs), gs[len(gs)-1].CreatedAt, gs[0].CreatedAt)
		return len(gs), gs[len(gs)-1].CreatedAt
	}
	lastFull, firstEmpty := 0, 0
	for _, p := range []int{1, 10, 30, 31, 100, 300, 1000, 3000, 10000} {
		n, _ := show(p)
		if n < 0 {
			return fmt.Errorf("probe stopped")
		}
		if n == 0 {
			firstEmpty = p
			break
		}
		lastFull = p
	}
	if firstEmpty == 0 {
		fmt.Println("page 10000 still has gists; the listing goes at least 1,000,000 deep")
		return nil
	}
	for firstEmpty-lastFull > 1 {
		m := (lastFull + firstEmpty) / 2
		n, _ := show(m)
		if n < 0 {
			return fmt.Errorf("probe stopped")
		}
		if n == 0 {
			firstEmpty = m
		} else {
			lastFull = m
		}
	}
	_, oldest := show(lastFull)
	fmt.Printf("\nlast page with gists: %d, so the API lists about %d gists, back to %s\n", lastFull, lastFull*100, oldest)
	return nil
}

// apiFetch lists the account's gists through the API, newest first, until it reaches gists
// created before from (or maxPages, or the end), adds them to the index, then downloads the raw
// logs in range that are not on disk yet, newest first, at most max of them.
func apiFetch(dir, from string, maxPages, workers, limit int) error {
	tok, err := token()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		return err
	}
	idxPath := filepath.Join(dir, "index.jsonl")
	idx, err := readIndex(idxPath)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(idxPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	listed, added, known := 0, 0, 0
	var oldest string
	for p := 1; maxPages == 0 || p <= maxPages; p++ {
		gs, err := apiPage(tok, p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "listing stopped: %v\n", err)
			break
		}
		if len(gs) == 0 {
			fmt.Printf("listing ends at page %d\n", p)
			break
		}
		past := false
		for _, g := range gs {
			if from != "" && g.CreatedAt < from {
				past = true
				continue
			}
			listed++
			oldest = g.CreatedAt
			if _, ok := idx[g.ID]; ok {
				known++
				continue
			}
			e := indexEntry{ID: g.ID, Created: g.CreatedAt, Listed: now}
			for _, f := range g.Files {
				e.File = f.Filename
			}
			idx[g.ID] = e
			added++
			j, _ := json.Marshal(e)
			out.Write(append(j, '\n'))
		}
		if p%10 == 0 {
			fmt.Printf("  page %d: back to %s, %d new so far\n", p, oldest, added)
		}
		if past {
			fmt.Printf("reached gists older than %s at page %d\n", from, p)
			break
		}
	}
	fmt.Printf("listed %d gists in range (oldest %s), %d new, %d already indexed; index holds %d\n", listed, oldest, added, known, len(idx))
	return download(dir, idx, workers, from, limit)
}

// ---- parse ----

// Mod is one line of "Loaded mods:".
type Mod struct {
	Name       string   `json:"name"`
	Pkg        string   `json:"pkg"` // lower-case
	MV         string   `json:"mv,omitempty"`
	Assemblies []string `json:"asm,omitempty"`
}

// HPatch is one line of "Active Harmony patches:".
type HPatch struct {
	Target  string   `json:"target"`
	Patches []string `json:"patches"` // "PRE: Ns.Type.Method" …
}

// Cluster is a cleaned error cluster.
type Cluster struct {
	Key     string `json:"key"`
	Class   string `json:"class"`
	Count   int    `json:"count"`
	Message string `json:"msg"`
	Top     string `json:"top,omitempty"`   // first stack frame type.method
	Level   string `json:"level,omitempty"` // 1.7 only: WARN | ERROR
}

// Record is one parsed log.
type Record struct {
	ID        string         `json:"id"`
	Created   string         `json:"created"`
	Hash      string         `json:"hash"`
	Status    string         `json:"status"` // ok | dropped
	Drop      string         `json:"drop,omitempty"`
	Format    string         `json:"format,omitempty"` // unity (≤1.6) | waylith (1.7)
	Levels    map[string]int `json:"levels,omitempty"` // waylith only: lines per level
	Version   string         `json:"version,omitempty"`
	MM        string         `json:"mm,omitempty"`
	Truncated bool           `json:"truncated,omitempty"`
	LogLines  int            `json:"log_lines"`
	Mods      []Mod          `json:"mods,omitempty"`
	Harmony   []HPatch       `json:"harmony,omitempty"`
	HarmonyV  string         `json:"harmony_versions,omitempty"`
	Clusters  []Cluster      `json:"clusters,omitempty"`
}

var (
	modRe = regexp.MustCompile(`\(([^()\s]+)\)(?:\[mv:([^\]]*)\])?: (.*)$`)
	hpRe  = regexp.MustCompile(`(PRE|post|TRANS|FIN|ILM): `)
	mmRe  = regexp.MustCompile(`^(\d+\.\d+)\.`)
)

func parse(id, created string, raw []byte) Record {
	sum := sha256.Sum256(raw)
	r := Record{ID: id, Created: created, Hash: hex.EncodeToString(sum[:8]), Status: "dropped"}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	head, body, found := strings.Cut(text, "\nLog file contents:\n")
	if !found {
		// The unofficial "Log Publisher from HugsLib" omits the marker before a 1.7 log.
		if i := strings.Index(text, "\n=== Waylith log ===\n"); i >= 0 {
			head, body, found = text[:i], text[i+1:], true
		}
	}
	if !found {
		r.Drop = "no-log-section"
		return r
	}
	if strings.TrimSpace(body) == "" {
		r.Drop = "empty-log" // shared with the mod list only
		return r
	}
	waylithVersion := ""
	var lineLevel map[int]string
	if strings.HasPrefix(body, "=== Waylith log ===\n") {
		r.Format = "waylith"
		body, waylithVersion, r.Levels, lineLevel = fromWaylith(body)
	} else {
		r.Format = "unity"
	}
	lines := strings.Split(head, "\n")
	sec := ""
	for _, l := range lines {
		switch {
		case l == "Loaded mods:":
			sec = "mods"
			continue
		case l == "Active Harmony patches:":
			sec = "harmony"
			continue
		case strings.HasPrefix(l, "Harmony versions present:"):
			r.HarmonyV = strings.TrimSpace(strings.TrimPrefix(l, "Harmony versions present:"))
			sec = ""
			continue
		case strings.HasPrefix(l, "Platform information:"):
			sec = "" // never kept (design §5 "明确不收集")
			continue
		}
		switch sec {
		case "mods":
			if l == "" {
				continue
			}
			m := modRe.FindStringSubmatchIndex(l)
			if m == nil {
				continue
			}
			mod := Mod{Name: strings.TrimSpace(l[:m[0]]), Pkg: strings.ToLower(l[m[2]:m[3]])}
			if m[4] >= 0 {
				mod.MV = l[m[4]:m[5]]
			}
			if asm := l[m[6]:m[7]]; asm != "(no assemblies)" {
				for _, a := range splitTop(asm) {
					mod.Assemblies = append(mod.Assemblies, a)
				}
			}
			r.Mods = append(r.Mods, mod)
		case "harmony":
			target, rest, ok := strings.Cut(l, ": ")
			if !ok {
				continue
			}
			idx := hpRe.FindAllStringIndex(": "+rest, -1)
			s := ": " + rest
			var ps []string
			for k, ix := range idx {
				end := len(s)
				if k+1 < len(idx) {
					end = idx[k+1][0]
				}
				ps = append(ps, strings.TrimSpace(s[ix[0]:end]))
			}
			r.Harmony = append(r.Harmony, HPatch{Target: target, Patches: ps})
		}
	}
	if len(r.Mods) == 0 {
		r.Drop = "no-mod-list"
		return r
	}
	r.Truncated = strings.Contains(body[max(0, len(body)-300):], "(log trimmed to ")
	lf, err := logs.Parse(strings.NewReader(body))
	if err != nil {
		r.Drop = "log-parse-error"
		return r
	}
	r.LogLines = lf.Lines
	r.Version = lf.Version
	if r.Version == "" {
		r.Version = waylithVersion
	}
	if m := mmRe.FindStringSubmatch(r.Version); m != nil {
		r.MM = m[1]
	}
	if r.MM == "" {
		r.Drop = "no-game-version"
		return r
	}
	for _, c := range logs.Clusters(lf) {
		cl := Cluster{Key: c.Key, Class: c.Class, Count: c.Count, Message: trunc(c.First.Message, 300), Level: lineLevel[c.First.Line]}
		if len(c.First.Frames) > 0 {
			cl.Top = c.First.Frames[0].Type + "." + c.First.Frames[0].Method
		}
		r.Clusters = append(r.Clusters, cl)
	}
	r.Status = "ok"
	return r
}

var (
	waylithLineRe = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d+ +([A-Z]+) \[[^\]]*\] ?(.*)$`)
	waylithVerRe  = regexp.MustCompile(`^Version:\s+(\d+\.\d+\.\d+)\.(\d+)`)
)

// fromWaylith turns a 1.7 (Waylith runtime) log into the Unity-style text internal/logs reads.
//   - The timestamp, level and category prefix is removed. INFO/DEBUG/TRACE entries are dropped
//     with their continuation lines, so only WARN and ERROR entries remain.
//   - A message can span lines; unindented continuation lines are indented so internal/logs
//     keeps them in the entry ("Objects: 1" would otherwise become an entry of its own).
//   - Every 1.7 entry carries a stack; its leading frames inside the logging system
//     (Brutal.Logging.*, Verse.Log.*) are removed so the first frame is the caller.
//   - Blank lines inside an entry are dropped (internal/logs ends an entry at a blank line).
//
// It returns the text, the game version from the header ("1.7.4982 rev1280", since the
// RimWorld line comes too late for internal/logs), the line count per level, and the level of
// each entry keyed by its line number in the returned text.
func fromWaylith(body string) (string, string, map[string]int, map[int]string) {
	var b strings.Builder
	ver := ""
	levels := map[string]int{}
	lineLevel := map[int]string{}
	out := 0 // lines written
	write := func(l string) {
		b.WriteString(l)
		b.WriteByte('\n')
		out++
	}
	skip := true // the header block until the first entry
	leading := false
	for _, l := range strings.Split(body, "\n") {
		if m := waylithLineRe.FindStringSubmatch(l); m != nil {
			levels[m[1]]++
			switch m[1] {
			case "WARN", "WARNING", "ERROR", "FATAL", "CRITICAL":
				skip, leading = false, true
				write("") // a blank line ends the previous entry
				write(m[2])
				lineLevel[out] = m[1]
			default:
				skip = true
			}
			continue
		}
		if ver == "" {
			if m := waylithVerRe.FindStringSubmatch(l); m != nil {
				ver = m[1] + " rev" + m[2]
			}
		}
		if skip || strings.TrimSpace(l) == "" {
			continue
		}
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "at ") {
			if leading && (strings.HasPrefix(t, "at Brutal.Logging.") || strings.HasPrefix(t, "at Verse.Log.") || strings.HasPrefix(t, "at .Verse.Log.")) {
				continue
			}
			leading = false
			write(l)
			continue
		}
		if l[0] != ' ' && l[0] != '\t' {
			l = "    " + l
		}
		write(l)
	}
	return b.String(), ver, levels, lineLevel
}

// splitTop splits "A(1.0), B(av:1,fv:2)" on commas outside parentheses.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ---- stats ----

func stats(dir string) error {
	idx, err := readIndex(filepath.Join(dir, "index.jsonl"))
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(idx))
	for id := range idx {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(a, b int) bool { return idx[ids[a]].Created < idx[ids[b]].Created })

	out, err := os.Create(filepath.Join(dir, "parsed.jsonl"))
	if err != nil {
		return err
	}
	defer out.Close()
	var recs []Record
	seenHash := map[string]bool{}
	drops := map[string]int{}
	missing := 0
	t0 := time.Now()
	for _, id := range ids {
		b, err := os.ReadFile(rawPath(dir, id))
		if err != nil {
			missing++
			continue
		}
		r := parse(id, idx[id].Created, b)
		if r.Status == "ok" && seenHash[r.Hash] {
			r.Status, r.Drop = "dropped", "duplicate-content"
		}
		seenHash[r.Hash] = true
		if r.Status == "ok" && r.MM != "1.6" && r.MM != "1.7" {
			r.Status, r.Drop = "dropped", "version-out-of-scope"
		}
		if r.Status != "ok" {
			drops[r.Drop]++
		}
		j, _ := json.Marshal(r)
		out.Write(append(j, '\n'))
		recs = append(recs, r)
	}
	parseTime := time.Since(t0)

	var ok []Record
	mm := map[string]int{}
	formats := map[string]int{}
	for _, r := range recs {
		if r.MM != "" {
			mm[r.MM]++
		}
		if r.Format != "" {
			formats[r.Format]++
		}
		if r.Status == "ok" {
			ok = append(ok, r)
		}
	}
	p := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }
	p("== corpus")
	p("indexed %d, raw on disk %d, missing %d; parse %v", len(idx), len(recs), missing, parseTime.Round(time.Millisecond))
	if len(ids) > 0 {
		p("upload window: %s .. %s", idx[ids[0]].Created, idx[ids[len(ids)-1]].Created)
	}
	p("game versions (all parsed): %s", fmtCounts(mm, 0))
	p("log formats: %s", fmtCounts(formats, 0))
	p("== cleaning")
	p("kept %d, dropped %d: %s", len(ok), len(recs)-len(ok), fmtCounts(drops, 0))
	if len(ok) == 0 {
		return nil
	}

	var nMods, nHar, nClus, withErr, withHar []int
	tr := 0
	classes := map[string]int{}
	levelClass := map[string]int{}
	classLogs := map[string]int{}
	pkgFreq := map[string]int{}
	clusterFreq := map[string]int{}
	clusterSample := map[string]Cluster{}
	for _, r := range ok {
		nMods = append(nMods, len(r.Mods))
		nHar = append(nHar, len(r.Harmony))
		nClus = append(nClus, len(r.Clusters))
		if r.Truncated {
			tr++
		}
		if len(r.Clusters) > 0 {
			withErr = append(withErr, 1)
		}
		if len(r.Harmony) > 0 {
			withHar = append(withHar, 1)
		}
		seenC := map[string]bool{}
		for _, c := range r.Clusters {
			classes[c.Class]++
			if c.Level != "" {
				levelClass[c.Level+" "+c.Class]++
			}
			if !seenC[c.Class] {
				classLogs[c.Class]++
				seenC[c.Class] = true
			}
			clusterFreq[c.Key]++
			if _, ok := clusterSample[c.Key]; !ok {
				clusterSample[c.Key] = c
			}
		}
		for _, m := range r.Mods {
			pkgFreq[m.Pkg]++
		}
	}
	p("== content of kept logs (%d)", len(ok))
	p("mods per log: %s", dist(nMods))
	p("harmony-patched methods per log: %s; logs with a Harmony section %d", dist(nHar), len(withHar))
	p("error clusters per log: %s; logs with at least one cluster %d (%.0f%%)", dist(nClus), len(withErr), pct(len(withErr), len(ok)))
	p("truncated logs: %d (%.0f%%)", tr, pct(tr, len(ok)))
	p("clusters by class: %s", fmtCounts(classes, 0))
	p("logs containing class: %s", fmtCounts(classLogs, 0))
	p("1.7 clusters by level and class: %s", fmtCounts(levelClass, 0))
	type kv struct {
		k string
		v int
	}
	var cf []kv
	for k, v := range clusterFreq {
		cf = append(cf, kv{k, v})
	}
	sort.Slice(cf, func(a, b int) bool { return cf[a].v > cf[b].v || cf[a].v == cf[b].v && cf[a].k < cf[b].k })
	p("distinct cluster keys %d; top 25 by number of logs:", len(cf))
	for i := 0; i < len(cf) && i < 25; i++ {
		c := clusterSample[cf[i].k]
		p("  %4d logs  %-13s %s", cf[i].v, c.Class, trunc(strings.ReplaceAll(c.Message, "\n", " "), 120))
	}

	// Mod coverage: how many distinct packages cover most entries (design §5 L2).
	var pf []int
	total := 0
	for _, v := range pkgFreq {
		pf = append(pf, v)
		total += v
	}
	sort.Sort(sort.Reverse(sort.IntSlice(pf)))
	p("== packages")
	p("distinct packageIds %d, mod entries %d", len(pf), total)
	for _, n := range []int{100, 300, 1000, 3000} {
		s := 0
		for i := 0; i < n && i < len(pf); i++ {
			s += pf[i]
		}
		p("  top %d packages cover %.0f%% of entries", n, pct(s, total))
	}

	// Sessions: repeat uploads by the same player (design §6.2) — same game version, package
	// sets with Jaccard >= 0.9, within 48 hours of the previous upload in the chain.
	p("== sessions (repeat uploads)")
	sets := make([]map[string]bool, len(ok))
	for i, r := range ok {
		sets[i] = map[string]bool{}
		for _, m := range r.Mods {
			sets[i][m.Pkg] = true
		}
	}
	parent := make([]int, len(ok))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	type pair struct{ a, b int }
	var links []pair
	for j := range ok {
		tj, _ := time.Parse(time.RFC3339, ok[j].Created)
		best, bestJ := -1, 0.0
		for i := j - 1; i >= 0; i-- {
			ti, _ := time.Parse(time.RFC3339, ok[i].Created)
			if tj.Sub(ti) > 48*time.Hour {
				break
			}
			if ok[i].MM != ok[j].MM {
				continue
			}
			if jac := jaccard(sets[i], sets[j]); jac >= 0.9 && jac > bestJ {
				best, bestJ = i, jac
			}
		}
		if best >= 0 {
			links = append(links, pair{best, j})
			parent[find(j)] = find(best)
		}
	}
	size := map[int]int{}
	for i := range ok {
		size[find(i)]++
	}
	inMulti, multi := 0, 0
	for _, s := range size {
		if s > 1 {
			multi++
			inMulti += s
		}
	}
	p("sessions with 2+ uploads: %d, covering %d logs (%.0f%%)", multi, inMulti, pct(inMulti, len(ok)))
	// Candidate fix evidence: consecutive uploads where the list changed a little and a cluster vanished.
	small, withVanish := 0, 0
	for _, l := range links {
		d := diffSize(sets[l.a], sets[l.b])
		if d == 0 || d > 3 {
			continue
		}
		small++
		before := map[string]bool{}
		for _, c := range ok[l.a].Clusters {
			before[c.Key] = true
		}
		after := map[string]bool{}
		for _, c := range ok[l.b].Clusters {
			after[c.Key] = true
		}
		for k := range before {
			if !after[k] {
				withVanish++
				break
			}
		}
	}
	p("consecutive pairs %d; with 1-3 package changes %d; of those, some cluster vanished %d", len(links), small, withVanish)
	return nil
}

func jaccard(a, b map[string]bool) float64 {
	in := 0
	for k := range a {
		if b[k] {
			in++
		}
	}
	u := len(a) + len(b) - in
	if u == 0 {
		return 0
	}
	return float64(in) / float64(u)
}

func diffSize(a, b map[string]bool) int {
	d := 0
	for k := range a {
		if !b[k] {
			d++
		}
	}
	for k := range b {
		if !a[k] {
			d++
		}
	}
	return d
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}

func dist(v []int) string {
	if len(v) == 0 {
		return "-"
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	q := func(f float64) int { return s[int(f*float64(len(s)-1))] }
	return fmt.Sprintf("min %d, p25 %d, median %d, p75 %d, p95 %d, max %d", s[0], q(.25), q(.5), q(.75), q(.95), s[len(s)-1])
}

func fmtCounts(m map[string]int, limit int) string {
	type kv struct {
		k string
		v int
	}
	var l []kv
	for k, v := range m {
		l = append(l, kv{k, v})
	}
	sort.Slice(l, func(a, b int) bool { return l[a].v > l[b].v || l[a].v == l[b].v && l[a].k < l[b].k })
	var parts []string
	for i, e := range l {
		if limit > 0 && i >= limit {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", e.k, e.v))
	}
	return strings.Join(parts, ", ")
}
