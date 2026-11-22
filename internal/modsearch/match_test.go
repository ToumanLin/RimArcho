package modsearch

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// searchText runs one matcher over text written to a Defs file.
func searchText(t *testing.T, ctx context.Context, text string, o Options) fileHits {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.xml")
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
	if o.Pattern == "" {
		o.Pattern = "needle"
	}
	o.FS = OS
	m, err := newMatcher(o)
	if err != nil {
		t.Fatal(err)
	}
	var scratch []byte
	return m.search(ctx, fileJob{path: path, rel: "Defs/a.xml", kind: Defs}, o, &scratch)
}

func TestLineModeStopsAtPerFile(t *testing.T) {
	text := "x needle y needle\nnone\nneedle z\nlast needle\n"
	want := []Match{{Line: 1, Col: 3, Text: "x needle y needle"}, {Line: 3, Col: 1, Text: "needle z"}}
	for _, regex := range []bool{false, true} {
		got := searchText(t, context.Background(), text, Options{LineMode: true, Regex: regex, PerFile: 2})
		if got.err != nil || got.count != 2 || !reflect.DeepEqual(got.matches, want) || !reflect.DeepEqual(got.offsets, []int{2, 23}) {
			t.Fatal(regex, got)
		}
	}
	// Without LineMode the count keeps going past PerFile.
	if got := searchText(t, context.Background(), text, Options{Regex: true, PerFile: 2}); got.count != 3 || len(got.matches) != 2 {
		t.Fatal(got)
	}
}

func TestLineModeSnippetKeepsMatchVisible(t *testing.T) {
	line := strings.Repeat("a", 250) + "needle" + strings.Repeat("b", 50)
	got := searchText(t, context.Background(), line, Options{LineMode: true, PerFile: 5})
	if len(got.matches) != 1 || got.matches[0].Text != "..."+line[197:] || got.matches[0].Col != 251 {
		t.Fatal(got.matches)
	}
	got = searchText(t, context.Background(), line, Options{PerFile: 5})
	if len(got.matches) != 1 || got.matches[0].Text != line[:157]+"..." {
		t.Fatal(got.matches)
	}
}

func TestSearchCancelledInEveryLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, o := range []Options{{}, {Regex: true}, {Regex: true, LineMode: true}} {
		if got := searchText(t, ctx, "needle\n", o); got.err != context.Canceled || got.count != 0 || got.matches != nil {
			t.Fatal(o, got)
		}
	}
}
