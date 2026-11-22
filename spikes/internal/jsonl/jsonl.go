// Package jsonl reads the corpus spikes' JSON-lines inputs. A path may name several files
// separated by commas (the weekly feature files of the corpus pipeline), and a file ending in
// .gz is decompressed on the fly. Lines are passed in file order.
package jsonl

import (
	"bufio"
	"compress/gzip"
	"io"
	"os"
	"strings"
)

// Paths splits a comma-separated path list, dropping empty parts.
func Paths(list string) []string {
	var out []string
	for _, p := range strings.Split(list, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Each calls f with every line of every file in list.
func Each(list string, f func([]byte) error) error {
	for _, p := range Paths(list) {
		if err := eachFile(p, f); err != nil {
			return err
		}
	}
	return nil
}

func eachFile(path string, f func([]byte) error) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	var src io.Reader = fh
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		zr, err := gzip.NewReader(fh)
		if err != nil {
			return err
		}
		defer zr.Close()
		src = zr
	}
	r := bufio.NewReaderSize(src, 1<<20)
	for {
		b, err := r.ReadBytes('\n')
		if len(b) > 0 {
			if e := f(b); e != nil {
				return e
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
