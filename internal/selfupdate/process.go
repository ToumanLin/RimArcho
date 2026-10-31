package selfupdate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// VersionOf runs an isolated version probe, with a fresh data directory and a 20 s timeout.
func VersionOf(ctx context.Context, exe string) (result string, err error) {
	home, err := os.MkdirTemp("", "rimarcho-version-*")
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(home)) }()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "version")
	cmd.Env = append(os.Environ(), "RIMARCHO_HOME="+home)
	hideWindow(cmd)
	var out firstLineWriter
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("version check: %w", err)
	}
	words := strings.Fields(string(out.line))
	if out.overflow || len(words) < 2 || words[0] != "archo" {
		return "", errors.New("version check returned an invalid first line")
	}
	if _, ok := Compare(words[1], words[1]); !ok {
		return "", errors.New("version check returned an invalid version")
	}
	return words[1], nil
}

// Only the first line matters; discard the rest without buffering unbounded output.
type firstLineWriter struct {
	line     []byte
	finished bool
	overflow bool
}

func (w *firstLineWriter) Write(p []byte) (int, error) {
	n := len(p)
	if !w.finished {
		if i := bytes.IndexByte(p, '\n'); i >= 0 {
			p, w.finished = p[:i], true
		}
		if len(w.line)+len(p) > 64<<10 {
			w.overflow, w.finished = true, true
		} else {
			w.line = append(w.line, p...)
		}
	}
	return n, nil
}
