// Package oplog is the operation log (02 §1, 04 §4, 13 §1.3): an append-only record of the
// actions RimArcho took on the player's files, from every surface, including refusals and
// failures. It is evidence, not authority: nothing reads it to decide an action, and a row
// that cannot be written never fails the action it describes.
package oplog

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ToumanLin/RimArcho/internal/store"
)

// Op codes (stable English; the surfaces translate them).
const (
	Trash              = "trash"
	Delete             = "delete" // deleted for good, skipping the recycle bin (quick delete)
	SubscriptionRemove = "subscription.remove"
	Unsubscribe        = "workshop.unsubscribe"
	ModsConfigWrite    = "modsconfig.write"
	LinkCreate         = "link.create"
	LinkRemove         = "link.remove"
	SaveRename         = "save.rename"
	SaveCopy           = "save.copy"
	SaveBackup         = "save.backup"
	SaveTrash          = "save.trash"
	UpdateApply        = "update.apply"
	UpdateRollback     = "update.rollback"
	Uninstall          = "uninstall"
	Launch             = "launch"
	PlanSync           = "plan.sync"        // a plan caught up with a followed collection
	LooksChoice        = "looks.choice"     // a look was applied from a Mod, or that choice undone (21 §3.3)
	LooksBuild         = "looks.build"      // the generated Mod RimArcho Looks Preference was rebuilt (21 §4)
	LooksStyle         = "looks.style"      // a style or random variant entry was added or removed (21 §5)
	StylesBuild        = "looks.stylebuild" // the generated Mod RimArcho Styles Preference was rebuilt (21 §5)
)

// Codes lists every op code, in the order the filters show them.
var Codes = []string{Launch, ModsConfigWrite, LinkCreate, LinkRemove, Trash, Delete, Unsubscribe, SubscriptionRemove, SaveRename, SaveCopy, SaveBackup, SaveTrash, UpdateApply, UpdateRollback, Uninstall, PlanSync, LooksChoice, LooksBuild, LooksStyle, StylesBuild}

// Permanent begins the detail of a subscription.remove whose folder was deleted for good
// (quick delete) instead of moved to the recycle bin.
const Permanent = "permanently deleted"

// Results.
const (
	OK      = "ok"
	Failed  = "failed"
	Refused = "refused"
	Partial = "partial"
	// Started: a parent row whose action has not ended (or whose process ended first).
	Started = "started"
)

// Origins: which surface's process wrote the row (13 §1.3).
const (
	GUI   = "gui"
	TUI   = "tui"
	CLI   = "cli"
	MCP   = "mcp"
	Agent = "agent"
	Auto  = "auto"
)

// Origins lists every origin, in the order the filters show them.
var Origins = []string{GUI, TUI, CLI, MCP, Agent, Auto}

// EnvVar carries the origin to child processes: a process that finds it set takes it as its
// own, so a scan the window starts writes gui rows. Archo's launcher sets it to agent for the
// MCP server it starts (internal/app/agent.go).
const EnvVar = "ARCHO_ORIGIN"

// Retention: rows older than MaxAge, and all but the newest MaxRows, are pruned.
const (
	MaxAge  = 90 * 24 * time.Hour
	MaxRows = 500
)

// Row is one recorded action.
type Row struct {
	ID      int64     `json:"id"`
	At      time.Time `json:"at"`
	Origin  string    `json:"origin"`
	EnvID   string    `json:"env,omitempty"`
	Op      string    `json:"op"`
	Subject string    `json:"subject,omitempty"`
	Target  string    `json:"target,omitempty"`
	Result  string    `json:"result"`
	Detail  string    `json:"detail,omitempty"`
	Batch   int64     `json:"batch,omitempty"`
}

// Log writes and reads op_log. A nil *Log does nothing and reads nothing.
type Log struct {
	db     *sql.DB
	mu     sync.Mutex
	origin string
	// Now is the clock (tests replace it); nil means time.Now.
	Now func() time.Time
	// Fail makes every write fail (tests of "a failed write never fails the action").
	Fail bool
}

// New is the log of the store s; origin "" is cli.
func New(s *store.Store, origin string) *Log {
	if s == nil || s.DB == nil {
		return nil
	}
	l := &Log{db: s.DB}
	l.SetOrigin(origin)
	return l
}

// OriginFromEnv is the origin a process inherited (EnvVar), or def.
func OriginFromEnv(def string) string {
	v := strings.TrimSpace(os.Getenv(EnvVar))
	for _, o := range Origins {
		if v == o {
			return v
		}
	}
	return def
}

// SetOrigin sets the process origin of the rows written from now on.
func (l *Log) SetOrigin(o string) {
	if l == nil {
		return
	}
	if o == "" {
		o = CLI
	}
	l.mu.Lock()
	l.origin = o
	l.mu.Unlock()
}

// Origin is the process origin.
func (l *Log) Origin() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.origin
}

func (l *Log) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

// Add appends r (At, Origin and Result filled in when empty) and returns its id; 0 when the
// row could not be written, which goes to the process log and is otherwise ignored.
func (l *Log) Add(r Row) int64 {
	if l == nil || r.Op == "" {
		return 0
	}
	if r.At.IsZero() {
		r.At = l.now()
	}
	if r.Origin == "" {
		r.Origin = l.Origin()
	}
	if r.Result == "" {
		r.Result = OK
	}
	r.Detail = clip(r.Detail, 400)
	if l.Fail {
		log.Printf("operation log: cannot record %s: writes are off", r.Op)
		return 0
	}
	res, err := l.db.Exec(`INSERT INTO op_log(at, origin, env_id, op, subject, target, result, detail, batch) VALUES(?,?,?,?,?,?,?,?,?)`,
		r.At.UnixMilli(), r.Origin, r.EnvID, r.Op, r.Subject, r.Target, r.Result, r.Detail, r.Batch)
	if err != nil {
		log.Printf("operation log: cannot record %s: %v", r.Op, err)
		return 0
	}
	id, _ := res.LastInsertId()
	return id
}

// Handle is a multi-step action's parent row (launch, uninstall, a batch of saves): its
// children carry its id in batch. A nil Handle is valid; its children are plain rows.
type Handle struct {
	l    *Log
	id   int64
	done bool
}

// Begin writes a parent row with result started; End closes it.
func (l *Log) Begin(r Row) *Handle {
	if l == nil {
		return nil
	}
	r.Result = Started
	return &Handle{l: l, id: l.Add(r)}
}

// ID is the parent row's id (0 when it could not be written).
func (h *Handle) ID() int64 {
	if h == nil {
		return 0
	}
	return h.id
}

// Child appends a row belonging to h.
func (h *Handle) Child(r Row) int64 {
	if h == nil {
		return 0
	}
	r.Batch = h.id
	return h.l.Add(r)
}

// End sets the parent row's result and detail; the first End wins.
func (h *Handle) End(result, detail string) {
	if h == nil || h.done {
		return
	}
	h.done = true
	if h.id == 0 || h.l.Fail {
		return
	}
	if _, err := h.l.db.Exec(`UPDATE op_log SET result=?, detail=? WHERE id=?`, result, clip(detail, 400), h.id); err != nil {
		log.Printf("operation log: cannot close row %d: %v", h.id, err)
	}
}

// Query selects rows for Recent.
type Query struct {
	Since  time.Time // only rows at or after; zero: any
	Op     string    // an op code, or a family ("save", "link"); "": all
	Failed bool      // only failed, refused and partial rows
	Limit  int       // the newest Limit rows; 0: all
}

// Recent returns the rows q selects, newest first.
func (l *Log) Recent(q Query) ([]Row, error) {
	if l == nil {
		return nil, nil
	}
	where, args := []string{"1=1"}, []any{}
	if !q.Since.IsZero() {
		where, args = append(where, "at >= ?"), append(args, q.Since.UnixMilli())
	}
	if op := strings.TrimSuffix(q.Op, "."); op != "" {
		where, args = append(where, "(op = ? OR substr(op, 1, ?) = ?)"), append(args, op, len(op)+1, op+".")
	}
	if q.Failed {
		where = append(where, "result IN ('failed','refused','partial')")
	}
	text := `SELECT id, at, origin, env_id, op, subject, target, result, detail, batch FROM op_log WHERE ` + strings.Join(where, " AND ") + ` ORDER BY id DESC`
	if q.Limit > 0 {
		text += fmt.Sprintf(" LIMIT %d", q.Limit)
	}
	rows, err := l.db.Query(text, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		var at int64
		if err := rows.Scan(&r.ID, &at, &r.Origin, &r.EnvID, &r.Op, &r.Subject, &r.Target, &r.Result, &r.Detail, &r.Batch); err != nil {
			return nil, err
		}
		r.At = time.UnixMilli(at)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Count is the number of rows.
func (l *Log) Count() int {
	if l == nil {
		return 0
	}
	var n int
	l.db.QueryRow(`SELECT COUNT(*) FROM op_log`).Scan(&n)
	return n
}

// Prune keeps the newer of the last MaxAge and the newest MaxRows rows: a row survives only
// when it is both younger than MaxAge and among the newest MaxRows.
func (l *Log) Prune() (int64, error) {
	if l == nil {
		return 0, nil
	}
	cut := l.now().Add(-MaxAge).UnixMilli()
	res, err := l.db.Exec(`DELETE FROM op_log WHERE at < ? OR id <= COALESCE((SELECT id FROM op_log ORDER BY id DESC LIMIT 1 OFFSET ?), 0)`, cut, MaxRows)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Failure reports whether a result is one the surfaces make stand out.
func Failure(result string) bool {
	return result == Failed || result == Refused || result == Partial
}

func clip(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", ""), "\n", "; "))
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
