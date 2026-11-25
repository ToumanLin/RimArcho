package app

import (
	"bytes"
	"io"
	"os"

	"github.com/ToumanLin/RimArcho/internal/analyze"
)

// LogSizeLimit is the largest Player.log read in full (06 §1).
const LogSizeLimit int64 = 64 << 20

const logWindow int64 = 4 << 20

type logSample struct {
	headEnd, tailStart, end int64
}

func (s logSample) omitted() int64 { return s.tailStart - s.headEnd }

func (s logSample) readers(f *os.File) (head, tail *io.SectionReader) {
	head = io.NewSectionReader(f, 0, s.headEnd)
	if s.tailStart < s.end {
		tail = io.NewSectionReader(f, s.tailStart, s.end-s.tailStart)
	}
	return head, tail
}

func logSections(f *os.File, size int64) (logSample, error) {
	if size <= LogSizeLimit {
		return logSample{size, size, size}, nil
	}
	head, err := logLineBoundary(f, 0, logWindow, true)
	if err != nil {
		return logSample{}, err
	}
	tail, err := logLineBoundary(f, size-logWindow, size, false)
	return logSample{head, tail, size}, err
}

// Align the inner edge to a complete line. If a whole window is one fragment,
// its bytes belong to the omitted middle.
func logLineBoundary(f *os.File, start, end int64, reverse bool) (int64, error) {
	var buf [16 << 10]byte
	for i := start; i < end; {
		n := min(end-i, int64(len(buf)))
		off := i
		if reverse {
			off = end - (i - start) - n
		}
		if _, err := f.ReadAt(buf[:n], off); err != nil {
			return 0, err
		}
		j := bytes.IndexByte(buf[:n], '\n')
		if reverse {
			j = bytes.LastIndexByte(buf[:n], '\n')
		}
		if j >= 0 {
			return off + int64(j) + 1, nil
		}
		i += n
	}
	if reverse {
		return start, nil
	}
	return end, nil
}

func readLogFailures(path string, size int64) ([]analyze.LogFailure, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	s, err := logSections(f, size)
	if err != nil {
		return nil, err
	}
	head, tail := s.readers(f)
	failures, err := analyze.ReadLogFailures(head)
	if err == nil && tail != nil {
		more, err := analyze.ReadLogFailures(tail)
		return append(failures, more...), err
	}
	return failures, err
}
