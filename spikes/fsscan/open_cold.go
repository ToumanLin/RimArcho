package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"strings"
	"time"
)

// Listing touches directory records only; it never opens file contents.
func listOpenFiles(root, ext string) ([]string, error) {
	var files []string
	stack := []string{root}
	storage := make([]uint64, 256*1024/8)
	for len(stack) > 0 {
		dir := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		err := listColdDirectory(dir, storage, func(name string, directory bool) {
			path := filepath.Join(dir, name)
			if directory {
				if !skipDir(name) {
					stack = append(stack, path)
				}
			} else if strings.EqualFold(filepath.Ext(name), ext) {
				files = append(files, path)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", dir, err)
		}
	}
	return files, nil
}

func runOpenCold(mods []string, workers []int, seed int64, groupMods, skipMods int, rewarm bool, ext string) error {
	if groupMods < 1 || skipMods < 0 {
		return fmt.Errorf("open-cold requires -groupmods >= 1 and -skipmods >= 0")
	}
	ext = strings.TrimSpace(ext)
	if len(ext) < 2 || ext[0] != '.' || strings.ContainsAny(ext, `/\;,*?`) {
		return fmt.Errorf("invalid -ext %q; use a single extension such as .png or .xml", ext)
	}
	// Division avoids overflow when validating large flag values.
	if skipMods > len(mods) || groupMods > (len(mods)-skipMods)/len(workers) {
		return fmt.Errorf("open-cold needs %d groups of %d mods after skipping %d; only %d mods available (adjust -limit, -groupmods, or -skipmods)", len(workers), groupMods, skipMods, len(mods))
	}
	shuffled := append([]string(nil), mods...)
	rand.New(rand.NewSource(seed)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	end := skipMods + groupMods*len(workers)
	groups := make([][]string, len(workers))
	start := time.Now()
	for g := range groups {
		for _, mod := range shuffled[skipMods+g*groupMods : skipMods+(g+1)*groupMods] {
			files, err := listOpenFiles(mod, ext)
			if err != nil {
				return err
			}
			groups[g] = append(groups[g], files...)
		}
	}
	fmt.Printf("open-cold listing (untimed): %.3f ms; ext=%s seed=%d shuffled_mods=[%d,%d) next_skipmods=%d\n", float64(time.Since(start))/float64(time.Millisecond), ext, seed, skipMods, end, end)
	fmt.Println("One pass per group; ms/file/worker = wall_ms * workers / files. Cold means first touch in this invocation; OS/Defender caches are retained.")
	printTable := func(label string) {
		fmt.Printf("open-cold %s\n", label)
		fmt.Printf("%7s %7s %10s %12s %12s %16s %8s %8s\n", "workers", "mods", "files", "wall_ms", "files/sec", "ms/file/worker", "errors", "short")
		for g, w := range workers {
			files := groups[g]
			start := time.Now()
			t := parallel(len(files), w, func(i int) totals {
				var t totals
				f, err := openColdFile(files[i])
				if err != nil {
					t.fail(fmt.Errorf("open %s: %w", files[i], err))
					return t
				}
				var header [33]byte
				_, err = io.ReadFull(f, header[:])
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					t.skipped++
				} else if err != nil {
					t.fail(fmt.Errorf("read %s: %w", files[i], err))
				}
				t.fail(f.Close())
				return t
			})
			elapsed := time.Since(start)
			ms := float64(elapsed) / float64(time.Millisecond)
			rate, perWorker := "n/a", "n/a"
			if len(files) > 0 {
				rate = fmt.Sprintf("%.1f", float64(len(files))/elapsed.Seconds())
				perWorker = fmt.Sprintf("%.6f", ms*float64(w)/float64(len(files)))
			}
			fmt.Printf("%7d %7d %10d %12.3f %12s %16s %8d %8d\n", w, groupMods, len(files), ms, rate, perWorker, t.errors, t.skipped)
			if t.firstError != "" {
				fmt.Printf("  first error: %s\n", t.firstError)
			}
		}
	}
	printTable("cold")
	if rewarm {
		printTable("warm")
	}
	return nil
}
