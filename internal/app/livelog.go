package app

import (
	"bufio"
	"bytes"
	"hash/maphash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Live log tracking (03 §6.6): follow Player.log while the game runs, reading only what was
// appended, keeping clusters instead of text.

const (
	liveMaxClusters = 2000
	liveSamples     = 3
	liveMaxRead     = 8 << 20 // bytes per poll; the rest comes on the next one
	liveMaxQuiet    = 50000   // remembered Player.log lines that are not errors (~1 MB)
)

// LiveCluster is one error cluster of a running game.
type LiveCluster struct {
	Phases               []logs.PhaseCount
	playerPhases         []logs.PhaseCount
	Key, Template, Class string
	Count                int
	FirstLine            int
	At                   time.Time // the companion log's timestamp, else when RimArcho saw it
	Level                string    // the companion log's level, if any
	// OnlyCompanion: the entry is in the companion log only (the game had stopped writing
	// Player.log, 03 §6.7.4).
	OnlyCompanion bool
	withComp      bool   // attributed with its companion entry
	redo          bool   // attribute again: Player.log showed a companion-only entry after all
	match         string // the companion entry a companion-only cluster started from
	alias         string // the Player.log key that joined a companion-only cluster
	// Report is the attribution of the first sample (roles, evidence, next steps); nil
	// until attributed.
	Report  *LogCluster
	samples []*logs.Entry
}

// LiveLog follows one Player.log.
type LiveLog struct {
	Path      string
	Companion string // the companion log followed: ArchoMonitor.log or RimCrow's ("" for none)
	Version   string
	Lines     int
	Resets    int // times the file started over (the game restarted)
	BadLines  int // companion lines that were not JSON

	dataRoot string // automatic pairing for FollowRun
	session  string
	marked   string

	off      int64
	pending  []byte
	p        *logs.Parser
	by       map[string]*LiveCluster
	byMatch  map[string]bool         // normalised first lines of the clusters' entries
	compOnly map[string]*LiveCluster // companion-only clusters by their entry's first line
	// quiet holds hashes of the normalised first lines of Player.log entries that are not
	// errors (noise, information): a monitor entry with the same line is in Player.log, as
	// the full analysis decides, and does not become a cluster of its own.
	quiet    map[uint64]struct{}
	order    []*LiveCluster
	behind   bool // Player.log has more left than one poll reads
	comp     *logs.Companion
	compRead map[*logs.CompanionEntry]time.Time // when each companion entry was first read
	compDone map[*logs.CompanionEntry]bool      // matched, or made a cluster of its own
	now      func() time.Time
	memo     map[string]liveKey // message + top frame → classification (repeats are common)
}

// liveCompanionWait is how long a companion entry waits for Player.log to show it before it
// becomes a cluster of its own (03 §6.7.4).
const liveCompanionWait = 5 * time.Second

type liveKey struct {
	interesting          bool
	key, template, class string
	quiet                uint64 // quietHash of the first line, for entries that are not errors
}

var quietSeed = maphash.MakeSeed()

func quietHash(match string) uint64 { return maphash.String(quietSeed, match) }

// FollowLog starts following path from its beginning (a running game's log is short),
// with an explicit companion log ("" for none).
func (a *App) FollowLog(path, companion string) *LiveLog {
	lv := &LiveLog{Path: path, now: a.Now}
	lv.reset()
	lv.follow(companion)
	return lv
}

// FollowRun follows a run using the same pairing rules as full analysis (03 §6.7.4).
// dataRoot may name a pack's save folder; empty uses the Player.log's data root.
func (a *App) FollowRun(path, dataRoot string) *LiveLog {
	if dataRoot == "" {
		dataRoot = logDataRoot(path)
	}
	lv := a.FollowLog(path, "")
	lv.dataRoot = dataRoot
	lv.session, lv.marked = logMarker(path)
	lv.refreshCompanion()
	return lv
}

func (lv *LiveLog) refreshCompanion() {
	if lv.dataRoot == "" {
		return
	}
	c := findCompanion(lv.Path, lv.dataRoot, lv.session, lv.marked)
	c = uniqueCompanion(lv.Path, lv.dataRoot, c, lv.session)
	if c != lv.Companion {
		lv.follow(c)
	}
}

// RunDataRoot is the save data root a run writes into: the pack's when a pack runs.
func (a *App) RunDataRoot(st *LoadoutState, w RunWhat) string {
	if w.UserData != "" {
		return w.UserData
	}
	return st.Ix.Env.UserDataRoot
}

func (lv *LiveLog) follow(companion string) {
	lv.Companion, lv.comp = companion, nil
	lv.compRead, lv.compDone = map[*logs.CompanionEntry]time.Time{}, map[*logs.CompanionEntry]bool{}
	if companion != "" {
		lv.comp = logs.NewCompanion(companion)
	}
}

func (lv *LiveLog) reset() {
	lv.off, lv.pending, lv.Lines = 0, nil, 0
	lv.p = logs.NewParser()
	lv.by = map[string]*LiveCluster{}
	lv.byMatch = map[string]bool{}
	lv.compOnly = map[string]*LiveCluster{}
	lv.quiet = map[uint64]struct{}{}
	lv.order = nil
	lv.memo = map[string]liveKey{}
}

// marker re-evaluates the session, including a monitor rotated since the marker was written.
func (lv *LiveLog) marker(line string) {
	m := logs.SessionRe.FindStringSubmatch(line)
	if m == nil {
		return
	}
	lv.session, lv.marked = m[2], strings.TrimSpace(m[3])
	if lv.dataRoot != "" {
		lv.refreshCompanion()
	} else if c := CompanionFor(lv.Path); c != lv.Companion {
		lv.follow(c)
	}
}

// classify is logs.Interesting + logs.ClusterKey, memoised on the exact message and top
// frame (the regular expressions cost ~70 µs an entry).
func (lv *LiveLog) classify(e *logs.Entry) liveKey {
	m := e.Message
	if top := e.Top(); top != nil {
		m += "\x00" + top.Type + "." + top.Method
	}
	if len(e.Extra) > 0 {
		m += "\x00" + strings.Join(e.Extra, "\x00")
	}
	if k, ok := lv.memo[m]; ok {
		return k
	}
	k := liveKey{interesting: logs.Interesting(e)}
	if k.interesting {
		k.key, k.template, k.class = logs.ClusterKeyHint(e, logs.HintFrom(lv.comp))
	} else {
		k.quiet = quietHash(firstLineKey(e.Message))
	}
	if len(lv.memo) >= 10000 {
		clear(lv.memo)
	}
	lv.memo[m] = k
	return k
}

// Clusters are the clusters so far, in order of first occurrence.
func (lv *LiveLog) Clusters() []*LiveCluster { return lv.order }

// CompanionLog is the companion log read so far (nil when none is followed).
func (lv *LiveLog) CompanionLog() *logs.Companion { return lv.comp }

// Poll reads what was appended since the last poll and returns the clusters that appeared.
func (lv *LiveLog) Poll(a *App) ([]*LiveCluster, error) {
	if lv.foreign(a) {
		return nil, nil
	}
	fi, err := os.Stat(lv.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // the game has not written it yet
		}
		return nil, err
	}
	if fi.Size() < lv.off {
		lv.reset()
		lv.session, lv.marked = logMarker(lv.Path)
		lv.Resets++
	}
	lv.refreshCompanion()
	// The companion first: its class hints apply to the entries read below.
	if lv.comp != nil {
		before := lv.comp.BadLines
		got, err := lv.comp.Poll(liveMaxRead)
		lv.BadLines += lv.comp.BadLines - before
		if err != nil {
			return nil, err
		}
		for _, e := range got {
			lv.compRead[e] = lv.now()
		}
	}
	var fresh []*LiveCluster
	if fi.Size() > lv.off {
		f, err := os.Open(lv.Path)
		if err != nil {
			return nil, err
		}
		n := min(fi.Size()-lv.off, liveMaxRead)
		buf := make([]byte, n)
		_, err = f.ReadAt(buf, lv.off)
		f.Close()
		if err != nil && err != io.EOF {
			return nil, err
		}
		lv.off += n
		lv.pending = append(lv.pending, buf...)
		if i := bytes.LastIndexByte(lv.pending, '\n'); i >= 0 {
			sc := bufio.NewScanner(bytes.NewReader(lv.pending[:i+1]))
			sc.Buffer(make([]byte, 64<<10), 64<<20)
			for sc.Scan() {
				l := sc.Text()
				if strings.Contains(l, "[Archo Monitor]") {
					lv.marker(l)
				}
				lv.p.Line(l)
			}
			lv.pending = append([]byte(nil), lv.pending[i+1:]...)
		}
		fresh = lv.take(false)
	}
	lv.behind = fi.Size() > lv.off
	lv.Lines, lv.Version = lv.p.F.Lines, lv.p.F.Version
	fresh = append(fresh, lv.companionOnly(false)...)
	return fresh, lv.finish(a, fresh)
}

// Close ends the log (the game exited): the last entry is complete.
func (lv *LiveLog) Close(a *App) ([]*LiveCluster, error) {
	if lv.foreign(a) {
		return nil, nil
	}
	if len(lv.pending) > 0 {
		lv.p.Line(string(lv.pending))
		lv.pending = nil
	}
	lv.p.Close()
	fresh := lv.take(true)
	fresh = append(fresh, lv.companionOnly(true)...)
	return fresh, lv.finish(a, fresh)
}

// Recheck on every read: a shared file can be replaced by another game's log while
// being followed. Discard earlier clusters too, so they cannot remain in live counts.
func (lv *LiveLog) foreign(a *App) bool {
	if a.Store == nil {
		return false
	}
	st, err := a.Loadout()
	if err != nil {
		return false // standalone log readers may have no registered environment
	}
	if a.CheckLogOwner(st, lv.Path) != nil {
		lv.reset()
		lv.follow("")
		return true
	}
	return false
}

// take moves the complete entries out of the parser into clusters.
func (lv *LiveLog) take(all bool) []*LiveCluster {
	es := lv.p.F.Entries
	k := len(es)
	if !all && lv.p.Open() {
		k--
	}
	if k <= 0 {
		return nil
	}
	var fresh []*LiveCluster
	for _, e := range es[:k] {
		ck := lv.classify(e)
		if !ck.interesting {
			if lv.comp != nil {
				if len(lv.quiet) >= liveMaxQuiet {
					clear(lv.quiet) // the monitor's entries meet their lines within seconds
				}
				lv.quiet[ck.quiet] = struct{}{}
			}
			continue
		}
		key, tmpl, cls := ck.key, ck.template, ck.class
		c := lv.by[key]
		fk := "" // normalising costs; repeats of a known cluster skip it
		if c == nil || c.OnlyCompanion {
			fk = firstLineKey(e.Message)
		}
		if c == nil {
			// The monitor's entry came first and waited out liveCompanionWait: Player.log
			// joins its cluster (the monitor's text can cluster under another key).
			if oc := lv.compOnly[fk]; oc != nil {
				c, oc.alias = oc, key
				lv.by[key] = oc
			}
		}
		if c == nil {
			c = &LiveCluster{Key: key, Template: tmpl, Class: cls, FirstLine: e.Line, At: lv.now()}
			lv.by[key] = c
			lv.order = append(lv.order, c)
			fresh = append(fresh, c)
			lv.byMatch[fk] = true
		}
		if c.OnlyCompanion {
			// One cluster, as if Player.log had come first: its entry leads, the counts start
			// over from Player.log (finish takes the monitor's count when it is larger).
			delete(lv.compOnly, c.match)
			c.OnlyCompanion, c.match, c.redo = false, "", true
			c.Count, c.FirstLine = 0, e.Line
			c.samples = append([]*logs.Entry{e}, c.samples[:min(len(c.samples), liveSamples-1)]...)
			lv.byMatch[fk] = true
		} else if len(c.samples) < liveSamples {
			c.samples = append(c.samples, e)
		}
		c.Count++
		c.playerPhases = logs.AddPhaseCount(c.playerPhases, e.Phase, 1)
	}
	lv.p.F.Entries = append([]*logs.Entry(nil), es[k:]...)
	lv.trim()
	return fresh
}

// trim drops the oldest informational clusters beyond the cap (03 §6.6).
func (lv *LiveLog) trim() {
	for len(lv.order) > liveMaxClusters {
		drop := -1
		for i, c := range lv.order {
			if logs.Informational(c.Class) {
				drop = i
				break
			}
		}
		if drop < 0 {
			drop = 0
		}
		d := lv.order[drop]
		delete(lv.by, d.Key)
		if d.alias != "" {
			delete(lv.by, d.alias)
		}
		if d.match != "" {
			delete(lv.compOnly, d.match)
		}
		lv.order = append(lv.order[:drop], lv.order[drop+1:]...)
	}
}

// companionOnly turns companion entries Player.log has not shown within
// liveCompanionWait (or at all, once the game exited) into clusters of their own: the game
// stops writing Player.log after its message limit, the monitor does not (03 §6.7.4).
func (lv *LiveLog) companionOnly(all bool) []*LiveCluster {
	if lv.comp == nil {
		return nil
	}
	// Player.log is still writing an entry, or has more to read than one poll takes: an
	// entry it may yet show keeps waiting (a slow next line left an open entry past the
	// wait, and its monitor entry became a second cluster; VM run 2026-10-28).
	open := ""
	if es := lv.p.F.Entries; !all && lv.p.Open() && len(es) > 0 {
		open = firstLineKey(es[len(es)-1].Message)
	}
	var fresh []*LiveCluster
	for _, e := range lv.comp.Entries {
		if lv.compDone[e] || !e.Monitor || (e.Level != "ERROR" && e.Level != "WARNING") {
			continue
		}
		if _, quiet := lv.quiet[quietHash(e.Match)]; quiet || lv.byMatch[e.Match] {
			lv.compDone[e] = true
			continue
		}
		if !all && (lv.behind || e.Match == open || lv.now().Sub(lv.compRead[e]) < liveCompanionWait) {
			continue
		}
		lv.compDone[e] = true
		en := logs.EntryOf(e)
		ck := lv.classify(en)
		if !ck.interesting {
			continue
		}
		c := lv.by[ck.key]
		if c == nil {
			c = &LiveCluster{Key: ck.key, Template: ck.template, Class: ck.class, At: e.At, Level: e.Level, OnlyCompanion: true, match: e.Match}
			lv.by[ck.key] = c
			lv.order = append(lv.order, c)
			fresh = append(fresh, c)
			lv.byMatch[e.Match] = true
			lv.compOnly[e.Match] = c
		}
		c.Count += max(e.Count, 1)
		if len(c.samples) < liveSamples {
			c.samples = append(c.samples, en)
		}
	}
	lv.trim()
	return fresh
}

// firstLineKey is how a cluster's message meets a companion entry: its first line, normalised.
func firstLineKey(m string) string {
	if i := strings.IndexByte(m, '\n'); i >= 0 {
		m = m[:i]
	}
	return logs.Normalize(strings.TrimSpace(m))
}

// finish attributes new clusters with the full-log engine and applies the companion log.
func (lv *LiveLog) finish(a *App, fresh []*LiveCluster) error {
	// A cluster attributed before its companion line was read is attributed again once the
	// line is there (the files and mods it names are evidence, 03 §6.7.4).
	// So is a companion-only cluster Player.log joined.
	todo := fresh
	for _, c := range lv.order {
		if c.Report != nil && len(c.samples) > 0 && (c.redo || !c.withComp && lv.comp.Lookup(c.samples[0].Message) != nil) {
			todo = append(todo[:len(todo):len(todo)], c)
		}
	}
	for _, c := range todo {
		c.withComp = len(c.samples) > 0 && lv.comp.Lookup(c.samples[0].Message) != nil
		c.redo = false
	}
	fresh = todo
	if len(fresh) > 0 {
		lf := &logs.File{Version: lv.Version, Waylith: lv.p.F.Waylith, HasPhases: lv.p.F.HasPhases}
		for _, c := range fresh {
			lf.Entries = append(lf.Entries, c.samples[0])
		}
		st, err := a.Loadout()
		if err != nil {
			return err
		}
		rep := &LogReport{Path: lv.Path, Label: filepath.Base(lv.Path), Version: lv.Version}
		st2, restore, err := a.logState(st, rep)
		if err != nil {
			return err
		}
		err = a.attribute(st2, rep, lf, nil, lv.comp)
		restore()
		if err != nil {
			return err
		}
		for i, c := range fresh {
			if i < len(rep.Clusters) && rep.Clusters[i].First == c.samples[0] {
				c.Report = rep.Clusters[i]
				continue
			}
			for _, lc := range rep.Clusters {
				if lc.First == c.samples[0] {
					c.Report = lc
				}
			}
		}
	}
	// Times, levels and counts from the companion log (03 §6.7.4).
	for _, c := range lv.order {
		if len(c.samples) == 0 {
			continue
		}
		c.Phases = nil
		if len(c.playerPhases) > 0 || lv.p.F.HasPhases || lv.comp != nil && lv.comp.HasPhases {
			// Rebuild from the independent sources. Player.log can arrive after its
			// companion count; incrementing the previously merged total would count
			// that occurrence twice and invent an unknown-phase remainder.
			c.Count = 0
			for _, pc := range c.playerPhases {
				c.Count += pc.Count
			}
		}
		if lv.p.F.HasPhases {
			c.Phases = append([]logs.PhaseCount(nil), c.playerPhases...)
		}
		if e := lv.comp.Lookup(c.samples[0].Message); e != nil {
			if c.Level == "" {
				c.At, c.Level = e.At, e.Level
			}
			if e.Count > c.Count {
				c.Count = e.Count
			}
			if lv.comp.HasPhases {
				c.Phases = logs.MergePhaseCounts(c.Phases, e.Phases)
			}
		}
		if lv.p.F.HasPhases || lv.comp != nil && lv.comp.HasPhases {
			n := 0
			for _, pc := range c.Phases {
				n += pc.Count
			}
			c.Count = max(c.Count, n)
			c.Phases = logs.CompletePhaseCounts(c.Phases, c.Count)
		}
		if c.Report != nil && c.Report.Cluster != nil {
			c.Report.Count = c.Count
			c.Report.Phases = c.Phases
			if lv.p.F.HasPhases && !c.OnlyCompanion {
				c.Report.Phase = c.samples[0].Phase
				if c.Report.Phase == nil {
					c.Report.Phase = logs.LoadingPhase
				}
			} else if e := lv.comp.Lookup(c.samples[0].Message); e != nil && lv.comp.HasPhases {
				c.Report.Phase = e.Phase
				if c.Report.Phase == nil {
					c.Report.Phase = logs.LoadingPhase
				}
			}
		}
	}
	return nil
}
