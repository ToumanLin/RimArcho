package modsearch

import (
	"context"
	"io/fs"
	"testing"
)

// cancelDirFS cancels the search once it has listed after directories.
type cancelDirFS struct {
	FS
	after  int
	cancel context.CancelFunc
}

func (f *cancelDirFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if f.after--; f.after < 0 {
		f.cancel()
	}
	return f.FS.ReadDir(path)
}

// ctxErrDirFS fails every listing with err.
type ctxErrDirFS struct {
	FS
	err error
}

func (f *ctxErrDirFS) ReadDir(string) ([]fs.DirEntry, error) { return nil, f.err }

func TestListFilesStopsOnCancel(t *testing.T) {
	m := fixture(t, map[string]string{"Defs/a.xml": "needle", "Defs/Sub/b.xml": "needle", "Patches/c.xml": "needle"})
	// Listings: root, Defs, Defs/Sub, Patches. Cancelling after the root stops before any scope folder
	// is resolved; after Defs, the walk of Defs stops and drops its files; after Defs/Sub, Patches is skipped.
	for after, files := range []int{0, -1, 2} {
		ctx, cancel := context.WithCancel(context.Background())
		l := listFiles(ctx, m, Options{FS: &cancelDirFS{FS: OS, after: after, cancel: cancel}, Kinds: Defs | Patches, AllVersions: true})
		cancel()
		if l.err != context.Canceled || files < 0 && l.files != nil || files >= 0 && len(l.files) != files {
			t.Fatal(after, l)
		}
		if l.unreadable != 0 || l.firstError != "" {
			t.Fatal(after, l)
		}
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded} {
		l := listFiles(context.Background(), m, Options{FS: &ctxErrDirFS{FS: OS, err: err}, Kinds: Defs, AllVersions: true})
		if l.err != err || l.unreadable != 0 || l.firstError != "" || l.files != nil {
			t.Fatal(err, l)
		}
	}
}
