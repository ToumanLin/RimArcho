package textures

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// todds (github.com/todds-encoder/todds, MPL-2.0, archived on 2026-03-31; 0.4.1 is the
// last release) is the encoder RimSort and RimCrow use.
const (
	ToddsVersion = "0.4.1"
	ToddsZipURL  = "https://github.com/todds-encoder/todds/releases/download/0.4.1/todds_Windows_0.4.1.zip"
	ToddsZipSize = 3846213 // bytes, as the release lists it
	ToddsHome    = "https://github.com/todds-encoder/todds"
)

// Encoder writes the DDS of each PNG next to it.
type Encoder interface {
	Encode(ctx context.Context, files []string, scale int, mips bool) error
}

// Todds runs todds.exe.
type Todds struct {
	Path    string
	Threads int             // 0: todds' default (all cores)
	Prepare func(*exec.Cmd) // hides the console window (set by internal/app)
	Log     io.Writer       // todds' output, when wanted
}

var versionRe = regexp.MustCompile(`(?m)^todds\s+(\d+\.\d+\.\d+)`)

// Version runs `todds --help` and reads its first line ("todds 0.4.1").
func (t *Todds) Version() (string, error) {
	cmd := exec.Command(t.Path, "--help")
	if t.Prepare != nil {
		t.Prepare(cmd)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := runCtx(ctx, cmd)
	if m := versionRe.FindStringSubmatch(out); m != nil {
		return m[1], nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", t.Path, err)
	}
	return "", fmt.Errorf("%s does not say it is todds", t.Path)
}

// Args are todds' arguments for one batch: BC1 for opaque textures, BC7 with alpha,
// flipped for the game (it uploads rows bottom first), sizes made a multiple of 4, always
// written (RimArcho decides what is stale), and the list of files.
func Args(list string, scale int, mips bool, threads int) []string {
	a := []string{"-f", "BC1", "-af", "BC7", "-vf", "-fs", "-o"}
	if !mips {
		a = append(a, "-nm")
	}
	if scale > 0 && scale < 100 {
		a = append(a, "-sc", fmt.Sprint(scale))
	}
	if threads > 0 {
		a = append(a, "-th", fmt.Sprint(min(threads, 32)))
	}
	return append(a, list)
}

// Encode runs todds over files (one PNG per line of a list file).
func (t *Todds) Encode(ctx context.Context, files []string, scale int, mips bool) error {
	if len(files) == 0 {
		return nil
	}
	lf, err := os.CreateTemp("", "rimarcho-todds-*.txt")
	if err != nil {
		return err
	}
	list := lf.Name()
	defer os.Remove(list)
	w := bufio.NewWriter(lf)
	for _, f := range files {
		fmt.Fprintln(w, f)
	}
	if err := w.Flush(); err != nil {
		lf.Close()
		return err
	}
	lf.Close()
	cmd := exec.Command(t.Path, Args(list, scale, mips, t.Threads)...)
	if t.Prepare != nil {
		t.Prepare(cmd)
	}
	out, err := runCtx(ctx, cmd)
	if t.Log != nil {
		io.WriteString(t.Log, out)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("todds: %w: %s", err, lastLine(out))
	}
	return nil
}

// runCtx runs cmd, killing it when ctx ends, and returns its combined output.
func runCtx(ctx context.Context, cmd *exec.Cmd) (string, error) {
	var buf strings.Builder
	var mu sync.Mutex
	pw := writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		if buf.Len() < 1<<20 {
			buf.Write(p)
		}
		return len(p), nil
	})
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-ctx.Done():
		cmd.Process.Kill()
		<-done
		return buf.String(), ctx.Err()
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func lastLine(s string) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(ls[len(ls)-1])
}

// ---- running a plan ----

// BatchSize is the most files given to one todds run (RimCrow uses 10,000; smaller batches
// report progress and stop sooner).
const BatchSize = 400

// Result is what became of one job.
type Result struct {
	Job
	OK      bool
	DDS     string
	DDSSize int64
	DDSTime int64 // unix nanoseconds
	Err     string
}

// Progress is reported after each batch.
type Progress struct {
	Done, Total int
	Bytes       int64 // DDS bytes written so far
}

// Run encodes the jobs in batches of one scale, checks every DDS it was meant to write and
// returns a result per job, in job order. Before each batch, before(jobs) is called (to
// record what is about to be written); after it, after(results). A stop (ctx) ends between
// or inside a batch; the results so far are returned with ctx's error.
func Run(ctx context.Context, enc Encoder, jobs []Job, o Options, before func([]Job) error, after func([]Result) error, progress func(Progress)) ([]Result, error) {
	o = o.Normal()
	_, exact := enc.(*Todds) // a test encoder writes headers only
	byScale := map[int][]int{}
	var scales []int
	for i, j := range jobs {
		if _, ok := byScale[j.Scale]; !ok {
			scales = append(scales, j.Scale)
		}
		byScale[j.Scale] = append(byScale[j.Scale], i)
	}
	sort.Ints(scales)
	res := make([]Result, len(jobs))
	for i, j := range jobs {
		res[i] = Result{Job: j, DDS: j.DDS(), Err: "not encoded"}
	}
	var p Progress
	p.Total = len(jobs)
	for _, sc := range scales {
		idx := byScale[sc]
		for len(idx) > 0 {
			n := min(BatchSize, len(idx))
			batch := idx[:n]
			idx = idx[n:]
			bj := make([]Job, n)
			files := make([]string, n)
			for k, i := range batch {
				bj[k], files[k] = jobs[i], jobs[i].Path
			}
			if before != nil {
				if err := before(bj); err != nil {
					return res, err
				}
			}
			start := time.Now().Add(-2 * time.Second)
			err := enc.Encode(ctx, files, sc, !o.NoMips)
			br := make([]Result, n)
			for k, i := range batch {
				res[i] = check(jobs[i], start, !o.NoMips, exact)
				br[k] = res[i]
				if res[i].OK {
					p.Bytes += res[i].DDSSize
				}
			}
			p.Done += n
			if after != nil {
				if aerr := after(br); aerr != nil && err == nil {
					err = aerr
				}
			}
			if progress != nil {
				progress(p)
			}
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				// todds failed for the batch: what it wrote was checked above; go on with
				// the rest, the failures are reported per file
				continue
			}
		}
	}
	return res, ctx.Err()
}

// check looks at the DDS a job should have written since start. exact: its size must be
// what DDSSize says (todds' output is; a run killed while writing leaves a shorter file).
func check(j Job, start time.Time, mips, exact bool) Result {
	r := Result{Job: j, DDS: j.DDS()}
	fi, err := os.Stat(r.DDS)
	if err != nil {
		r.Err = "no DDS written"
		return r
	}
	if fi.ModTime().Before(start) {
		r.Err = "the DDS was not rewritten"
		return r
	}
	if err := CheckDDS(r.DDS); err != nil {
		r.Err = err.Error()
		return r
	}
	if want := DDSSize(j.W, j.H, j.Alpha, j.Scale, mips); exact && fi.Size() != want {
		r.Err = fmt.Sprintf("the DDS is %d bytes, not %d (incomplete)", fi.Size(), want)
		return r
	}
	r.OK, r.DDSSize, r.DDSTime = true, fi.Size(), fi.ModTime().UnixNano()
	return r
}

// DDSInfo is what a DDS header says.
type DDSInfo struct {
	W, H, Mips int
	Format     string // "BC1", "BC3", "BC7", "raw"
}

// ReadDDS reads a DDS header the way the game checks it (Verse.ModDdsLoader): the magic,
// a 124-byte header, a 32-byte pixel format; formats it loads are DXT1, DXT5, BC7 (DX10
// header, DXGI 98 or 99) and uncompressed RGB(A).
func ReadDDS(path string) (DDSInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return DDSInfo{}, err
	}
	defer f.Close()
	b := make([]byte, 148)
	n, _ := io.ReadFull(f, b)
	if n < 128 || string(b[:4]) != "DDS " {
		return DDSInfo{}, errors.New("not a DDS file")
	}
	le := binary.LittleEndian
	if le.Uint32(b[4:]) != 124 || le.Uint32(b[76:]) != 32 {
		return DDSInfo{}, errors.New("a DDS header the game does not read")
	}
	in := DDSInfo{H: int(le.Uint32(b[12:])), W: int(le.Uint32(b[16:])), Mips: int(le.Uint32(b[28:]))}
	pfFlags := le.Uint32(b[80:])
	switch cc := string(b[84:88]); {
	case pfFlags&0x4 == 0:
		in.Format = "raw"
	case cc == "DXT1":
		in.Format = "BC1"
	case cc == "DXT5":
		in.Format = "BC3"
	case cc == "DX10" && n >= 148 && (le.Uint32(b[128:]) == 98 || le.Uint32(b[128:]) == 99):
		in.Format = "BC7"
	default:
		return in, fmt.Errorf("a DDS format the game does not load (%q)", cc)
	}
	return in, nil
}

// CheckDDS reports whether the game can load the DDS at path.
func CheckDDS(path string) error {
	_, err := ReadDDS(path)
	return err
}

// FindTodds looks for todds.exe in dirs (each a folder or the program itself).
func FindTodds(dirs ...string) string {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		for _, p := range []string{d, filepath.Join(d, "todds.exe"), filepath.Join(d, "todds")} {
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}
