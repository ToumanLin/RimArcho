package store

import (
	"database/sql"
	"encoding/binary"
	"errors"
	"sort"
	"sync"
	"unicode/utf8"

	"github.com/klauspost/compress/zstd"

	"github.com/ToumanLin/RimArcho/internal/scan"
)

// A snapshot's file manifest is split in two (04 §4, schema v28): snapshot_file keeps the
// rows that have readers by row (semantic files with their hashes, and every .dll, which log
// attribution looks up by name), and snapshot_list holds the rest (textures, sounds,
// Languages and so on) as one zstd block per snapshot: paths sorted, front-coded, with their
// sizes and no mtime. Readers must accept either place for any file (test fixtures and old
// tools may still write plain rows).

// listed reports whether a manifest file goes into the snapshot's block rather than a row.
func listed(rel string, hasHash bool) bool {
	return !hasHash && !hasSuffixFold(rel, ".dll")
}

// hasSuffixFold is strings.HasSuffix with ASCII letters folded, like SQLite's LIKE.
func hasSuffixFold(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	t := s[len(s)-len(suffix):]
	for i := 0; i < len(t); i++ {
		a, b := t[i], suffix[i]
		if 'A' <= a && a <= 'Z' {
			a += 'a' - 'A'
		}
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}

// listEntry is one file of a snapshot's block.
type listEntry struct {
	rel  string
	size int64
}

const listFormat = 1 // the first byte of an uncompressed block

var listCodec struct {
	once sync.Once
	enc  *zstd.Encoder
	dec  *zstd.Decoder
	err  error
}

// codec makes the zstd encoder and decoder on first use, with one worker each and small
// windows: blocks are a few KB, and page processes should not keep large buffers (16 §10).
func codec() (*zstd.Encoder, *zstd.Decoder, error) {
	c := &listCodec
	c.once.Do(func() {
		c.enc, c.err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault), zstd.WithEncoderConcurrency(1),
			zstd.WithWindowSize(1<<20), zstd.WithLowerEncoderMem(true))
		if c.err == nil {
			c.dec, c.err = zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(256<<20))
		}
	})
	return c.enc, c.dec, c.err
}

// encodeList compresses entries sorted by path (bytewise, the order of snapshot_file's key).
func encodeList(es []listEntry) ([]byte, error) {
	enc, _, err := codec()
	if err != nil {
		return nil, err
	}
	n := 1
	for _, e := range es {
		n += len(e.rel) + 6
	}
	b := make([]byte, 1, n)
	b[0] = listFormat
	prev := ""
	for _, e := range es {
		p := 0
		for p < len(prev) && p < len(e.rel) && prev[p] == e.rel[p] {
			p++
		}
		b = binary.AppendUvarint(b, uint64(p))
		b = binary.AppendUvarint(b, uint64(len(e.rel)-p))
		b = append(b, e.rel[p:]...)
		b = binary.AppendUvarint(b, uint64(e.size))
		prev = e.rel
	}
	return enc.EncodeAll(b, make([]byte, 0, len(b)/3+16)), nil
}

var errBadList = errors.New("store: damaged snapshot file list")

// decodeList calls fn for each entry of a block, in path order.
func decodeList(data []byte, fn func(rel string, size int64)) error {
	_, dec, err := codec()
	if err != nil {
		return err
	}
	b, err := dec.DecodeAll(data, nil)
	if err != nil {
		return err
	}
	if len(b) == 0 || b[0] != listFormat {
		return errBadList
	}
	b = b[1:]
	var prev []byte
	for len(b) > 0 {
		p, n := binary.Uvarint(b)
		if n <= 0 || p > uint64(len(prev)) {
			return errBadList
		}
		b = b[n:]
		l, n := binary.Uvarint(b)
		if n <= 0 || l > uint64(len(b)-n) {
			return errBadList
		}
		b = b[n:]
		cur := append(prev[:p:p], b[:l]...)
		b = b[l:]
		size, n := binary.Uvarint(b)
		if n <= 0 {
			return errBadList
		}
		b = b[n:]
		fn(string(cur), int64(size))
		prev = cur
	}
	return nil
}

// insertFiles writes a snapshot's manifest: its rows and its block.
func insertFiles(tx *sql.Tx, snapID int64, files []scan.File) error {
	st, err := tx.Prepare(`INSERT OR REPLACE INTO snapshot_file(snapshot_id, rel_path, size, mtime, hash) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	var es []listEntry
	for _, f := range files {
		if listed(f.Rel, f.HasHash) {
			es = append(es, listEntry{f.Rel, f.Size})
			continue
		}
		var h any
		if f.HasHash {
			h = f.Hash[:]
		}
		if _, err := st.Exec(snapID, f.Rel, f.Size, f.MTime, h); err != nil {
			return err
		}
	}
	return insertList(tx, snapID, es)
}

// insertList stores a block (none when it is empty). Like INSERT OR REPLACE on the rows, a
// path given twice keeps its last size.
func insertList(tx *sql.Tx, snapID int64, es []listEntry) error {
	if len(es) == 0 {
		return nil
	}
	sort.SliceStable(es, func(i, j int) bool { return es[i].rel < es[j].rel })
	out := es[:0]
	for i, e := range es {
		if i+1 < len(es) && es[i+1].rel == e.rel {
			continue
		}
		out = append(out, e)
	}
	data, err := encodeList(out)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT OR REPLACE INTO snapshot_list(snapshot_id, n, data) VALUES(?,?,?)`, snapID, len(out), data)
	return err
}

// copyFiles copies a snapshot's manifest, rows and block, to another snapshot.
func copyFiles(tx *sql.Tx, to, from int64) error {
	if _, err := tx.Exec(`INSERT INTO snapshot_file(snapshot_id, rel_path, size, mtime, hash)
		SELECT ?, rel_path, size, mtime, hash FROM snapshot_file WHERE snapshot_id=?`, to, from); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO snapshot_list(snapshot_id, n, data) SELECT ?, n, data FROM snapshot_list WHERE snapshot_id=?`, to, from)
	return err
}

// deleteFiles deletes the manifests of the snapshots selected by "snapshot_id <where>"
// (for example "IN (SELECT …)" or "=?") and returns how many files they listed.
func deleteFiles(tx *sql.Tx, where string, args ...any) (int64, error) {
	var n int64
	if err := tx.QueryRow(`SELECT coalesce(sum(n),0) FROM snapshot_list WHERE snapshot_id `+where, args...).Scan(&n); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM snapshot_list WHERE snapshot_id `+where, args...); err != nil {
		return 0, err
	}
	res, err := tx.Exec(`DELETE FROM snapshot_file WHERE snapshot_id `+where, args...)
	if err != nil {
		return 0, err
	}
	rows, err := res.RowsAffected()
	return n + rows, err
}

// likeMatch reports whether s matches the pattern as SQLite's built-in LIKE does without
// ESCAPE: % is any run of characters, _ one character, and only ASCII letters ignore case.
func likeMatch(p, s string) bool {
	star, mark := -1, 0
	i, j := 0, 0
	for j < len(s) {
		if i < len(p) {
			switch p[i] {
			case '%':
				star, mark = i, j
				i++
				continue
			case '_':
				_, w := utf8.DecodeRuneInString(s[j:])
				i, j = i+1, j+w
				continue
			default:
				pr, pw := utf8.DecodeRuneInString(p[i:])
				sr, sw := utf8.DecodeRuneInString(s[j:])
				if foldASCII(pr) == foldASCII(sr) {
					i, j = i+pw, j+sw
					continue
				}
			}
		}
		if star < 0 {
			return false
		}
		_, w := utf8.DecodeRuneInString(s[mark:])
		mark += w
		i, j = star+1, mark
	}
	for i < len(p) && p[i] == '%' {
		i++
	}
	return i == len(p)
}

func foldASCII(r rune) rune {
	if 'A' <= r && r <= 'Z' {
		return r + 'a' - 'A'
	}
	return r
}

// onlyDLLs: every path the pattern matches ends in .dll, so all of them are rows.
func onlyDLLs(like string) bool { return hasSuffixFold(like, ".dll") }
