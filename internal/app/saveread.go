package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/savefile"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// The full read of a save (12 §1.7 L1): savefile.Scan over the whole file, cached in
// save_cache by path, size, modification time and parser version. Lists carry only the
// summary (Save.Summary); the facts are loaded for one save at a time (SaveFacts). The GUI
// reads in the `archo saves index` child process; the CLI reads one save inline.

// SaveSummary is the part of a save's full read that lists and grouping use.
type SaveSummary struct {
	Ticks     int64  `json:"ticks"`
	Day       int    `json:"day"`               // whole in-game days since the start
	Identity  string `json:"-"`                 // world identity (12 §4.1); "" when the save holds none
	World     string `json:"world,omitempty"`   // world name
	Faction   string `json:"faction,omitempty"` // the player faction's name
	Malformed bool   `json:"malformed,omitempty"`
}

func summaryOf(r store.SaveSummaryRow) *SaveSummary {
	return &SaveSummary{Ticks: r.Ticks, Day: int(r.Ticks / savefile.TicksPerDay), Identity: r.Identity,
		World: r.World, Faction: r.Faction, Malformed: r.Malformed}
}

func summaryRow(f *savefile.Facts) store.SaveSummaryRow {
	return store.SaveSummaryRow{Ticks: f.TicksGame, Identity: worldIdentity(f), World: f.World.Name,
		Faction: f.Player.Name, Malformed: f.Malformed != nil}
}

// worldIdentity is the key of the world a save plays in: seed, persistent random value
// and world name. A world generated again from the same seed gets another persistent
// random value; the player faction's name is left out because the player renames it
// during play (research note 2026-11-04 §3).
func worldIdentity(f *savefile.Facts) string {
	w := f.World
	if w.Seed == "" && w.PersistentRandomValue == 0 {
		return ""
	}
	return w.Seed + "|" + strconv.Itoa(w.PersistentRandomValue) + "|" + strings.ToLower(w.Name)
}

func saveKey(s *Save) string { return env.PathKey(s.Path) }

// attachSummaries sets the summary of every save whose cache row is still valid (a row of
// an older parser that means the same is, savefile.MinParserVersion, so a parser change
// that only adds facts does not unread every save), and marks the saves whose stored read
// failed on the file as it is now.
func (a *App) attachSummaries(saves []*Save) error {
	rows, err := a.Store.SaveSummaries()
	if err != nil {
		return err
	}
	for _, s := range saves {
		if s.Writing || s.ModTime.IsZero() { // being written, or not even its size is known
			continue
		}
		r := rows[saveKey(s)]
		switch {
		case r.Valid(s.Size, s.ModTime.UnixNano(), savefile.MinParserVersion, savefile.ParserVersion):
			s.Summary = summaryOf(r.SaveSummaryRow)
		case r.Failed(s.Size, s.ModTime.UnixNano(), savefile.ParserVersion):
			s.ReadFailed = true
		}
	}
	return nil
}

// NeedsRead reports whether a full read (12 §1.7 L1) would read the save: no valid cache
// row, not being written, and no stored failure on the file as it is now.
func (s *Save) NeedsRead() bool {
	return s.Summary == nil && !s.Writing && !s.ReadFailed && !s.ModTime.IsZero()
}

// SaveFacts returns a save's full read from the cache. With read, a save without a valid
// cache row, or with a row of an older parser (MinParserVersion), is read now and stored
// (the CLI's one save, the page's selected one: tens of ms per 10 MB), even when an earlier
// read of it failed. Without read it may return an older parser's facts (Facts.Parser says
// which); nil when not cached or for a save being written (SaveReadFailure says why when a
// stored read failed).
func (a *App) SaveFacts(s *Save, read bool) (*savefile.Facts, error) {
	if s.Writing {
		return nil, nil
	}
	c, err := a.Store.SaveFacts(saveKey(s))
	if err != nil {
		return nil, err
	}
	if c.Valid(s.Size, s.ModTime.UnixNano(), savefile.MinParserVersion, savefile.ParserVersion) {
		var f savefile.Facts
		if json.Unmarshal(c.Facts, &f) == nil && (!read || f.Parser == savefile.ParserVersion) {
			s.Summary = summaryOf(c.SaveSummaryRow)
			return &f, nil
		}
	}
	if !read {
		return nil, nil
	}
	return a.readSave(s)
}

// SaveReadFailure is why the stored full read of the save as it is now failed; "" when it
// did not fail or the file changed since.
func (a *App) SaveReadFailure(s *Save) (string, error) {
	c, err := a.Store.SaveFacts(saveKey(s))
	if err != nil || !c.Failed(s.Size, s.ModTime.UnixNano(), savefile.ParserVersion) {
		return "", err
	}
	return string(c.Facts), nil
}

// readSave reads a whole save and stores its facts. A file that changes while it is read
// is being written: nil, not stored. A read that fails is stored as a failure of the file
// as it is (unless the failure passes by itself: the file is gone or another program
// holds it), so lists do not read it again until it changes.
func (a *App) readSave(s *Save) (*savefile.Facts, error) {
	f, before, err := scanSave(s.Path)
	if err != nil {
		if fi, serr := os.Stat(s.Path); serr == nil && !passingReadErr(err) {
			a.Store.PutSaveFailure(saveKey(s), fi.Size(), fi.ModTime().UnixNano(), savefile.ParserVersion, err.Error())
		}
		return nil, err
	}
	after, err := os.Stat(s.Path)
	if err != nil || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		s.Writing = true
		return nil, nil
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	row := summaryRow(f)
	if err := a.Store.PutSaveCache(&store.SaveCache{Path: saveKey(s), Size: before.Size(), MTime: before.ModTime().UnixNano(),
		Parser: savefile.ParserVersion, SaveSummaryRow: row, Facts: raw}); err != nil {
		return nil, err
	}
	s.Size, s.ModTime, s.Summary, s.ReadFailed = before.Size(), before.ModTime(), summaryOf(row), false
	return f, nil
}

// scanSave reads the whole file at path; before is its state when opened.
func scanSave(path string) (*savefile.Facts, os.FileInfo, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer fh.Close()
	before, err := fh.Stat()
	if err != nil {
		return nil, nil, err
	}
	f, err := savefile.Scan(bufio.NewReaderSize(fh, 256<<10))
	if err != nil {
		return nil, nil, err
	}
	return f, before, nil
}

// passingReadErr: the read failed for a reason that passes by itself (the file is gone, or
// another program holds it), so the failure is not stored.
func passingReadErr(err error) bool {
	return errors.Is(err, os.ErrNotExist) || sharingViolation(err)
}

// SaveIndexProgress is reported after each save is read (Err: why it could not be).
type SaveIndexProgress struct {
	Done, Total int
	Save        *Save
	Err         string
}

// SaveIndexResult is what IndexSaves did.
type SaveIndexResult struct {
	Read       int      // read now
	Cached     int      // already in the cache
	Writing    int      // being written: read on a later run
	Unreadable int      // an earlier read failed and the file is unchanged: not read again
	Bytes      int64    // bytes read
	Errs       []string // per save that could not be read now
	Elapsed    time.Duration
}

// IndexSaves reads the saves without a valid cache row in the order given, 2–4 at a
// time, storing each as soon as it is read, so a stopped run keeps what it read (12 §1.7).
// A save whose stored read failed is not read again while it is unchanged.
func (a *App) IndexSaves(saves []*Save, progress func(SaveIndexProgress)) (*SaveIndexResult, error) {
	start := time.Now()
	r := &SaveIndexResult{}
	var todo []*Save
	for _, s := range saves {
		switch {
		case s.Summary != nil:
			r.Cached++
		case s.Writing:
			r.Writing++
		case s.ReadFailed:
			r.Unreadable++
		default:
			todo = append(todo, s)
		}
	}
	var mu sync.Mutex
	done := 0
	work := make(chan *Save)
	var wg sync.WaitGroup
	for range min(SaveReaders(), len(todo)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range work {
				f, err := a.readSave(s)
				mu.Lock()
				p := SaveIndexProgress{Total: len(todo), Save: s}
				switch {
				case err != nil:
					r.Errs = append(r.Errs, s.Name+": "+err.Error())
					p.Err = err.Error()
				case f == nil:
					r.Writing++
				default:
					r.Read++
					r.Bytes += f.Bytes
				}
				done++
				if progress != nil {
					p.Done = done
					progress(p)
				}
				mu.Unlock()
			}
		}()
	}
	for _, s := range todo {
		work <- s
	}
	close(work)
	wg.Wait()
	r.Elapsed = time.Since(start)
	return r, nil
}

// SaveReaders is how many saves IndexSaves reads at once: half the CPUs, 2 to 4.
func SaveReaders() int { return max(2, min(4, runtime.NumCPU()/2)) }

// IndexOrder puts the saves named by first ahead (this list's copies of them), then every
// other save newest first.
func (l *SaveList) IndexOrder(first []*Save) []*Save {
	byKey := map[string]*Save{}
	for _, s := range l.Saves() {
		byKey[saveKey(s)] = s
	}
	var out []*Save
	seen := map[string]bool{}
	for _, s := range first {
		k := saveKey(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		if o := byKey[k]; o != nil {
			s = o
		}
		out = append(out, s)
	}
	rest := []*Save{}
	for _, s := range l.Saves() {
		if !seen[saveKey(s)] {
			rest = append(rest, s)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].ModTime.After(rest[j].ModTime) })
	return append(out, rest...)
}

// PruneSaveCache removes the cache rows of saves that are gone: not listed and no file at
// the path. It returns how many it removed.
func (a *App) PruneSaveCache(l *SaveList) (int, error) {
	rows, err := a.Store.SaveSummaries()
	if err != nil {
		return 0, err
	}
	listed := map[string]bool{}
	for _, s := range l.Saves() {
		listed[saveKey(s)] = true
	}
	var gone []string
	for p := range rows {
		if listed[p] {
			continue
		}
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			gone = append(gone, p)
		}
	}
	return len(gone), a.Store.DeleteSaveCache(gone)
}

// ---- the child process ----

// SavesIndexLine is the progress line `archo saves index --progress` prints per save; a
// save that could not be read gets a second line, "saves-failed <ref> <reason>".
func SavesIndexLine(p SaveIndexProgress) string {
	name := p.Save.Name
	if p.Save.Colony != nil {
		name = p.Save.Colony.Name + "/" + name
	}
	line := fmt.Sprintf("progress: saves %d/%d %s %s", p.Done, p.Total, p.Save.Ref, name)
	if p.Err != "" {
		line += "\nprogress: saves-failed " + p.Save.Ref + " " + strings.ReplaceAll(p.Err, "\n", " ")
	}
	return line
}

var (
	savesIndexLineRe = regexp.MustCompile(`^(?:progress: )?saves (\d+)/(\d+) (sv:[0-9a-f]+) (.*)$`)
	savesFailLineRe  = regexp.MustCompile(`^(?:progress: )?saves-failed (sv:[0-9a-f]+) (.*)$`)
)

// ParseSavesFailLine reads the "saves-failed" line of SavesIndexLine: the save and why it
// could not be read.
func ParseSavesFailLine(line string) (ref, reason string, ok bool) {
	m := savesFailLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// ParseSavesIndexLine reads a progress line of SavesIndexLine.
func ParseSavesIndexLine(line string) (done, total int, ref, name string, ok bool) {
	m := savesIndexLineRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return 0, 0, "", "", false
	}
	done, _ = strconv.Atoi(m[1])
	total, _ = strconv.Atoi(m[2])
	return done, total, m[3], m[4], true
}

// RunSavesIndex brings the full-read cache up to date with the configured runner (by
// default `archo saves index --progress` as a child process of exe, "" this executable),
// reading the saves named by first (references) ahead of the rest.
func (a *App) RunSavesIndex(exe string, first []string, progress func(string)) error {
	return a.RunSavesIndexContext(context.Background(), exe, first, progress)
}

// RunSavesIndexContext is RunSavesIndex that ends the child process when ctx is done
// (the GUI's stop and window close, 16 §2.16): each save it read is already stored. A
// configured in-process runner (tests) is not interrupted.
func (a *App) RunSavesIndexContext(ctx context.Context, exe string, first []string, progress func(string)) error {
	if a.SavesIndexRunner != nil {
		return a.SavesIndexRunner(first, progress)
	}
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return err
		}
		if b := strings.ToLower(filepath.Base(exe)); strings.HasSuffix(b, ".test") || strings.HasSuffix(b, ".test.exe") {
			return errors.New("saves index from a test binary: set App.SavesIndexRunner")
		}
	}
	return ChildSavesIndexContext(ctx, exe, a.Home, first, progress)
}

// ChildSavesIndex runs `<exe> saves index --progress [<first>…]` and streams its lines.
func ChildSavesIndex(exe, home string, first []string, progress func(string)) error {
	return ChildSavesIndexContext(context.Background(), exe, home, first, progress)
}

// ChildSavesIndexContext is ChildSavesIndex, killing the child when ctx is done.
func ChildSavesIndexContext(ctx context.Context, exe, home string, first []string, progress func(string)) error {
	cmd := exec.CommandContext(ctx, exe, append([]string{"saves", "index", "--progress"}, first...)...)
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return err
	}
	// why is the child's own words for a failure, not its closing "next:" hint: its first
	// error line, else its last line that is neither progress nor a hint
	var why, errLine string
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if progress != nil {
			progress(line)
		}
		switch {
		case line == "" || strings.HasPrefix(line, "progress:") || strings.HasPrefix(line, "next:"):
		case strings.HasPrefix(line, "error:"):
			if errLine == "" {
				errLine = strings.TrimSpace(strings.TrimPrefix(line, "error:"))
			}
		default:
			why = line
		}
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errLine != "" {
			why = errLine
		}
		return &SavesIndexError{Err: err, Why: why, Fatal: errLine != ""}
	}
	return nil
}

// SavesIndexError is a failed `archo saves index` child: its exit and its own words.
// Fatal: it ended on an "error:" line (the run itself failed), not only on saves it
// could not read, which it lists and exits non-zero for.
type SavesIndexError struct {
	Err   error
	Why   string
	Fatal bool
}

func (e *SavesIndexError) Error() string {
	if e.Why == "" {
		return "archo saves index: " + e.Err.Error()
	}
	return "archo saves index: " + e.Why
}

func (e *SavesIndexError) Unwrap() error { return e.Err }

// InProcessSavesIndex runs the indexing in this process (tests).
func InProcessSavesIndex(a *App) func([]string, func(string)) error {
	return func(first []string, progress func(string)) error {
		l, err := a.ListSaves()
		if err != nil {
			return err
		}
		var pick []*Save
		for _, ref := range first {
			s, err := l.Find(ref)
			if err != nil {
				return err
			}
			pick = append(pick, s)
		}
		_, err = a.IndexSaves(l.IndexOrder(pick), func(p SaveIndexProgress) {
			if progress != nil {
				progress(SavesIndexLine(p))
			}
		})
		return err
	}
}
