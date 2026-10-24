package gametest

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	consoleMessage = regexp.MustCompile(`^(\d\d:\d\d:\d\d)  (INFO|WARN|ERROR|DEBUG|TRACE|FATAL|CRIT) (.*)$`)
	consoleTime    = regexp.MustCompile(`^\d\d:\d\d:\d\d `)
	versionRev     = regexp.MustCompile(`^(\d+\.\d+\.\d+) rev(\d+)$`)
)

// consoleLog streams Waylith console output in the file format (08 §12).
type consoleLog struct {
	w       io.Writer
	date    string
	pending []byte
}

func newConsoleLog(w io.Writer, version string, now time.Time) (*consoleLog, error) {
	version = strings.TrimSpace(version)
	version = versionRev.ReplaceAllString(version, `${1}.${2}`)
	_, err := fmt.Fprintf(w, "=== Waylith log ===\nVersion:   %s\n\n", version)
	return &consoleLog{w: w, date: now.Format("2006-01-02")}, err
}

func (c *consoleLog) Write(p []byte) (int, error) {
	c.pending = append(c.pending, p...)
	for {
		i := bytes.IndexByte(c.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		if err := c.line(c.pending[:i]); err != nil {
			return 0, err
		}
		c.pending = c.pending[i+1:]
	}
}

func (c *consoleLog) flush() error {
	if len(c.pending) == 0 {
		return nil
	}
	err := c.line(c.pending)
	c.pending = nil
	return err
}

func (c *consoleLog) line(b []byte) error {
	line := strings.TrimSuffix(string(b), "\r")
	if m := consoleMessage.FindStringSubmatch(line); m != nil {
		_, err := fmt.Fprintf(c.w, "%s %s.000  %s %s\n", c.date, m[1], m[2], m[3])
		return err
	}
	line = consoleTime.ReplaceAllString(line, "")
	_, err := fmt.Fprintln(c.w, line)
	return err
}
