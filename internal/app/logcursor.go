package app

import (
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/logs"
)

// The live log over MCP (03 §6.6): get_log / `archo log` return a cursor; called again with
// since=<cursor> they list only the clusters that are new or whose count grew after it.
//
// The cursor holds no state on disk: it is how many bytes of the log (and of its companion
// log, 03 §6.7) the report covered and a checksum of those bytes. A later call checks that
// the log still starts with the same bytes — the game appends to Player.log while it runs and
// starts a new one when it restarts (the old one becomes Player-prev.log) — and clusters that
// prefix again, without attribution, to know each cluster's count at the cursor.

// LogCursor marks how far a report read a log and its companion.
type LogCursor struct {
	Size     int64  // bytes of the log the report covered
	Sum      uint32 // their CRC-32C
	CompSize int64  // bytes of the companion log (0: none was read)
	CompSum  uint32
}

const cursorPrefix = "log1."

// String is the cursor as a token: log1.<size>.<sum>[.<companion size>.<sum>], sizes in base 36.
func (c LogCursor) String() string {
	s := fmt.Sprintf("%s%s.%08x", cursorPrefix, strconv.FormatInt(c.Size, 36), c.Sum)
	if c.CompSize > 0 {
		s += fmt.Sprintf(".%s.%08x", strconv.FormatInt(c.CompSize, 36), c.CompSum)
	}
	return s
}

// ErrBadCursor: the since value is not a cursor get_log or archo log returned.
var ErrBadCursor = errors.New("not a log cursor (use the cursor archo log or get_log returned, e.g. log1.4fz.1a2b3c4d)")

// ParseLogCursor reads a cursor token.
func ParseLogCursor(s string) (LogCursor, error) {
	var c LogCursor
	p := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), cursorPrefix), ".")
	if !strings.HasPrefix(strings.TrimSpace(s), cursorPrefix) || (len(p) != 2 && len(p) != 4) {
		return c, ErrBadCursor
	}
	bad := false
	num := func(x string) int64 {
		n, err := strconv.ParseInt(x, 36, 64)
		bad = bad || err != nil || n < 0
		return n
	}
	sum := func(x string) uint32 {
		n, err := strconv.ParseUint(x, 16, 32)
		bad = bad || err != nil || len(x) != 8
		return uint32(n)
	}
	c.Size, c.Sum = num(p[0]), sum(p[1])
	if len(p) == 4 {
		c.CompSize, c.CompSum = num(p[2]), sum(p[3])
	}
	if bad {
		return LogCursor{}, ErrBadCursor
	}
	return c, nil
}

// logMark is how many bytes were read from a file and their checksum.
type logMark struct {
	size int64
	sum  uint32
}

type countingCRC struct {
	h interface {
		io.Writer
		Sum32() uint32
	}
	n int64
}

func (c *countingCRC) Write(b []byte) (int, error) {
	c.n += int64(len(b))
	return c.h.Write(b)
}

func newCRC() *countingCRC { return &countingCRC{h: crc32.New(castagnoli)} }

// readLog parses the first size bytes of a log (the size its cache key was taken at: the game
// may be appending) and marks them.
func readLog(path string, size int64) (*logs.File, logMark, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, logMark{}, err
	}
	defer f.Close()
	h := newCRC()
	lf, err := logs.Parse(io.TeeReader(io.LimitReader(f, size), h))
	if err != nil {
		return nil, logMark{}, err
	}
	io.Copy(h, io.LimitReader(f, size-h.n)) // whatever the parser left unread
	return lf, logMark{h.n, h.h.Sum32()}, nil
}

// readCompanion reads a companion log, its first size bytes (size < 0: all of it, up to
// 256 MiB as logs.ReadCompanion), and marks them.
func readCompanion(path string, size int64) (*logs.Companion, logMark) {
	f, err := os.Open(path)
	if err != nil {
		return nil, logMark{}
	}
	defer f.Close()
	if size < 0 {
		size = 256 << 20
	}
	b, err := io.ReadAll(io.LimitReader(f, size))
	if err != nil {
		return nil, logMark{}
	}
	c := logs.NewCompanion(path)
	c.Feed(b, true)
	return c, logMark{int64(len(b)), crc32.Checksum(b, castagnoli)}
}

// LogSince compares a report with a cursor.
type LogSince struct {
	// Stale says why the cursor does not apply to this log ("" when it does): the log was
	// replaced by a new run. Every cluster is then new.
	Stale string
	// Moved is where the cursor's log is now, when it was rotated to Player-prev.log.
	Moved string
	// New are the report's clusters that are new or whose count grew after the cursor.
	New []*LogCluster
	// Then is each cluster's count at the cursor (by cluster key).
	Then map[string]int
}

// Since compares a log report with a cursor an earlier report of the same log returned.
func (a *App) Since(rep *LogReport, cur LogCursor) (*LogSince, error) {
	out := &LogSince{Then: map[string]int{}}
	if rep.Path == "" {
		return nil, errors.New("since applies to a log file, not to pasted text")
	}
	lf, ok, err := prefixOf(rep.Path, cur.Size, cur.Sum)
	if err != nil {
		return nil, err
	}
	if !ok {
		out.Stale = fmt.Sprintf("%s is a new run: it no longer starts with what the cursor saw (the game restarted)", rep.Label)
		if strings.EqualFold(rep.Label, "Player.log") {
			prev := filepath.Join(filepath.Dir(rep.Path), "Player-prev.log")
			if _, ok, _ := prefixOf(prev, cur.Size, cur.Sum); ok {
				out.Moved = prev
				out.Stale = "Player.log is a new run (the game restarted); the run the cursor is from is now Player-prev.log"
			}
		} else if strings.Contains(strings.ToLower(rep.Label), "-prev") {
			out.Stale = fmt.Sprintf("%s was replaced by a newer run (the game restarted again)", rep.Label)
		}
		out.New = rep.Clusters
		return out, nil
	}
	// The companion as it was at the cursor: the same file's first CompSize bytes. A
	// companion that changed under the cursor (another session) counts as nothing known.
	var comp *logs.Companion
	if p := CompanionFor(rep.Path); p != "" && cur.CompSize > 0 {
		if c, m := readCompanion(p, cur.CompSize); c != nil && m.size == cur.CompSize && m.sum == cur.CompSum {
			comp = c
		}
	}
	out.Then = clusterCounts(lf, comp)
	for _, lc := range rep.Clusters {
		if lc.Count > out.Then[lc.Key] {
			out.New = append(out.New, lc)
		}
	}
	return out, nil
}

// prefixOf parses the first size bytes of a file when their checksum is sum (ok false when
// the file is shorter or starts differently).
func prefixOf(path string, size int64, sum uint32) (*logs.File, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || fi.Size() < size {
		return nil, false, err
	}
	h := newCRC()
	lf, err := logs.Parse(io.TeeReader(io.LimitReader(f, size), h))
	if err != nil {
		return nil, false, err
	}
	io.Copy(h, io.LimitReader(f, size-h.n))
	if h.n != size || h.h.Sum32() != sum {
		return nil, false, nil
	}
	return lf, true, nil
}

// clusterCounts is each cluster's count as the full analysis computes it, without the
// attribution: Player.log's entries plus the companion-only ones, clustered with the
// companion's class hints, the count raised to the companion's (03 §6.7.4).
func clusterCounts(lf *logs.File, comp *logs.Companion) map[string]int {
	companionOnly(lf, comp)
	out := map[string]int{}
	for _, c := range logs.ClustersHint(lf, logs.HintFrom(comp)) {
		n := c.Count
		if comp != nil {
			if e := comp.Lookup(c.First.Message); e != nil && e.Count > n {
				n = e.Count
			}
		}
		out[c.Key] = n
	}
	return out
}
