package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/logs"
)

// Live log tracking (10 §3): follow Player.log while the game runs, reading only what was
// appended, keeping clusters instead of text.

const (
	liveMaxClusters = 2000
	liveSamples     = 3
	liveMaxRead     = 8 << 20 // bytes per poll; the rest comes on the next one
)

// LiveCluster is one error cluster of a running game.
type LiveCluster struct {
	Key, Template, Class string
	Count                int
	FirstLine            int
	At                   time.Time // the companion log's timestamp, else when RimArcho saw it
	Level                string    // the companion log's level, if any
	// Report is the attribution of the first sample (roles, evidence, next steps); nil
	// until attributed.
	Report  *LogCluster
	samples []*logs.Entry
}

// LiveLog follows one Player.log.
type LiveLog struct {
	Path      string
	Companion string // RimCrow_Realtime.log ("" when absent)
	Version   string
	Lines     int
	Resets    int // times the file started over (the game restarted)
	BadLines  int // companion lines that were not JSON

	off     int64
	pending []byte
	p       *logs.Parser
	by      map[string]*LiveCluster
	order   []*LiveCluster
	comp    *companionLog
	now     func() time.Time
	memo    map[string]liveKey // message + top frame → classification (repeats are common)
}

type liveKey struct {
	interesting          bool
	key, template, class string
}

// FollowLog starts following path from its beginning (a running game's log is short).
// The RimCrow companion log next to the user data is read when present (10 §3.3).
func (a *App) FollowLog(path, companion string) *LiveLog {
	lv := &LiveLog{Path: path, Companion: companion, now: a.Now}
	lv.reset()
	if companion != "" {
		lv.comp = &companionLog{path: companion, seen: map[string]companionLine{}}
	}
	return lv
}

// CompanionLog is RimCrow's realtime log in the environment's user data ("" if absent).
func (a *App) CompanionLog(st *LoadoutState) string {
	if st.Ix.Env.UserDataRoot == "" {
		return ""
	}
	p := filepath.Join(st.Ix.Env.UserDataRoot, "RimCrow_Realtime.log")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

func (lv *LiveLog) reset() {
	lv.off, lv.pending, lv.Lines = 0, nil, 0
	lv.p = logs.NewParser()
	lv.by = map[string]*LiveCluster{}
	lv.order = nil
	lv.memo = map[string]liveKey{}
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
		k.key, k.template, k.class = logs.ClusterKey(e)
	}
	if len(lv.memo) >= 10000 {
		clear(lv.memo)
	}
	lv.memo[m] = k
	return k
}

// Clusters are the clusters so far, in order of first occurrence.
func (lv *LiveLog) Clusters() []*LiveCluster { return lv.order }

// Poll reads what was appended since the last poll and returns the clusters that appeared.
func (lv *LiveLog) Poll(a *App) ([]*LiveCluster, error) {
	fi, err := os.Stat(lv.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // the game has not written it yet
		}
		return nil, err
	}
	if fi.Size() < lv.off {
		lv.reset()
		lv.Resets++
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
				lv.p.Line(sc.Text())
			}
			lv.pending = append([]byte(nil), lv.pending[i+1:]...)
		}
		fresh = lv.take(false)
	}
	lv.Lines, lv.Version = lv.p.F.Lines, lv.p.F.Version
	if lv.comp != nil {
		bad, err := lv.comp.poll()
		lv.BadLines += bad
		if err != nil {
			return fresh, err
		}
	}
	return fresh, lv.finish(a, fresh)
}

// Close ends the log (the game exited): the last entry is complete.
func (lv *LiveLog) Close(a *App) ([]*LiveCluster, error) {
	if len(lv.pending) > 0 {
		lv.p.Line(string(lv.pending))
		lv.pending = nil
	}
	lv.p.Close()
	fresh := lv.take(true)
	return fresh, lv.finish(a, fresh)
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
			continue
		}
		key, tmpl, cls := ck.key, ck.template, ck.class
		c := lv.by[key]
		if c == nil {
			c = &LiveCluster{Key: key, Template: tmpl, Class: cls, FirstLine: e.Line, At: lv.now()}
			lv.by[key] = c
			lv.order = append(lv.order, c)
			fresh = append(fresh, c)
		}
		c.Count++
		if len(c.samples) < liveSamples {
			c.samples = append(c.samples, e)
		}
	}
	lv.p.F.Entries = append([]*logs.Entry(nil), es[k:]...)
	lv.trim()
	return fresh
}

// trim drops the oldest informational clusters beyond the cap (10 §3.2).
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
		delete(lv.by, lv.order[drop].Key)
		lv.order = append(lv.order[:drop], lv.order[drop+1:]...)
	}
}

// finish attributes new clusters with the full-log engine and applies the companion log.
func (lv *LiveLog) finish(a *App, fresh []*LiveCluster) error {
	if len(fresh) > 0 {
		lf := &logs.File{Version: lv.Version, Waylith: lv.p.F.Waylith}
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
		err = a.attribute(st2, rep, lf, nil)
		restore()
		if err != nil {
			return err
		}
		byKey := map[string]*LogCluster{}
		for _, lc := range rep.Clusters {
			byKey[lc.Key] = lc
		}
		for _, c := range fresh {
			c.Report = byKey[c.Key]
		}
	}
	if lv.comp != nil {
		st, err := a.Loadout()
		if err != nil {
			return err
		}
		for _, c := range lv.order {
			if c.Level != "" || len(c.samples) == 0 {
				continue
			}
			cl, ok := lv.comp.seen[logs.Normalize(c.samples[0].Message)]
			if !ok {
				continue
			}
			c.At, c.Level = cl.at, cl.level
			if c.Report != nil {
				for _, pkg := range cl.related {
					mentionPackage(st, c.Report, pkg)
				}
			}
		}
	}
	for _, c := range lv.order {
		if c.Report != nil && c.Report.Cluster != nil {
			c.Report.Count = c.Count
		}
	}
	return nil
}

// mentionPackage adds an enabled instance with packageId pkg as "mentioned" by the
// companion mod, unless the cluster already names it.
func mentionPackage(st *LoadoutState, lc *LogCluster, pkg string) {
	pkg = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(pkg), "_steam"))
	for _, e := range st.List {
		m := st.Ix.ByID[e.Instance]
		if m == nil || m.Status != identity.Live || m.PackageID != pkg {
			continue
		}
		for _, at := range lc.Attr {
			if at.Instance == m.ID {
				return
			}
		}
		lc.Attr = append(lc.Attr, &Attribution{Instance: m.ID, Role: RoleMentioned, Evidence: []string{"RimCrow companion: relatedModIds " + pkg}})
		return
	}
}

// companionLog reads RimCrow_Realtime.log: one JSON object per line (10 §3.3).
type companionLog struct {
	path    string
	off     int64
	pending []byte
	seen    map[string]companionLine // normalised message → first occurrence
}

type companionLine struct {
	at      time.Time
	level   string
	related []string
}

func (c *companionLog) poll() (bad int, err error) {
	fi, err := os.Stat(c.path)
	if err != nil {
		return 0, nil
	}
	if fi.Size() < c.off {
		c.off, c.pending = 0, nil
		clear(c.seen)
	}
	if fi.Size() == c.off {
		return 0, nil
	}
	f, err := os.Open(c.path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := min(fi.Size()-c.off, liveMaxRead)
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, c.off); err != nil && err != io.EOF {
		return 0, err
	}
	c.off += n
	c.pending = append(c.pending, buf...)
	i := bytes.LastIndexByte(c.pending, '\n')
	if i < 0 {
		return 0, nil
	}
	for _, line := range bytes.Split(c.pending[:i], []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var j struct {
			Timestamp, Level, Message string
			Context                   struct {
				RelatedModIDs []string `json:"relatedModIds"`
			}
		}
		if json.Unmarshal(line, &j) != nil || j.Message == "" {
			bad++
			continue
		}
		key := logs.Normalize(j.Message)
		if _, ok := c.seen[key]; ok {
			continue
		}
		if len(c.seen) >= 20000 {
			continue
		}
		at, _ := time.ParseInLocation("2006-01-02 15:04:05.000", j.Timestamp, time.Local)
		c.seen[key] = companionLine{at: at, level: strings.ToUpper(j.Level), related: j.Context.RelatedModIDs}
	}
	c.pending = append([]byte(nil), c.pending[i+1:]...)
	return bad, nil
}
