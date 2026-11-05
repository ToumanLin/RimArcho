package store

import (
	"context"
)

// TruncateWAL checkpoints the write-ahead log and truncates it to zero bytes, after a write
// that may have grown it far past journal_size_limit (an analysis, a scan that wrote new
// snapshots). It does not wait: while another connection still reads an older state, the
// checkpoint stops early and the next reset truncates the file instead (04 §4).
func (s *Store) TruncateWAL() {
	ctx := context.Background()
	c, err := s.DB.Conn(ctx)
	if err != nil {
		return
	}
	defer c.Close()
	c.ExecContext(ctx, `PRAGMA busy_timeout=0`)
	c.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	c.ExecContext(ctx, `PRAGMA busy_timeout=5000`)
}
