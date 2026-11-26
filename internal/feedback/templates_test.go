package feedback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 16 §2.14: six templates in the page's order, each question key unique within its template,
// and a files-lost bug is the only priority.
func TestTemplates(t *testing.T) {
	if got := strings.Join(Kinds(), ","); got != "feature,bug,diagnosis,data,l10n,other" {
		t.Fatalf("kinds: %s", got)
	}
	for _, tp := range Templates {
		seen := map[string]bool{}
		for _, q := range tp.Questions {
			if seen[q.Key] || q.Label == "" || q.Form == "" {
				t.Fatalf("%s: question %+v", tp.Kind, q)
			}
			seen[q.Key] = true
		}
	}
	if TemplateOf("nope").Kind != KindOther {
		t.Fatal("an unknown kind is other")
	}
	if !Priority(KindBug, map[string]string{"files": "yes"}) || Priority(KindFeature, map[string]string{"files": "yes"}) || Priority(KindBug, nil) {
		t.Fatal("priority: a bug with files lost only")
	}
	got := AnswersText([]Answer{{"what you did", "x"}, {"empty", " "}, {"", "free words"}}, []string{"files were changed or lost"})
	if got != "**files were changed or lost**\n\n**what you did**\nx\n\nfree words" {
		t.Fatalf("answers:\n%s", got)
	}
}

// What each template ticks first (16 §2.14 "Templates", 2026-11-26).
func TestTemplateAttach(t *testing.T) {
	want := map[string]string{KindFeature: "", KindBug: "log,ops", KindDiagnosis: "mods,problems", KindData: "mods,problems", KindL10n: "", KindOther: "ops"}
	for _, tp := range Templates {
		if got := strings.Join(tp.Attach, ","); got != want[tp.Kind] {
			t.Errorf("%s ticks %q, want %q", tp.Kind, got, want[tp.Kind])
		}
	}
}

// A translation fix's codes: the one each line or ;-part starts with, once each, lowercased;
// words and bare text are not codes.
func TestRefCodes(t *testing.T) {
	got := RefCodes(`hom22 → 시작
LIB3: x; fbk40 = y
hom22 again
hello world
abc
run7x → no
  edt5
verylongpfx1 z
ab123456789 → too many digits`)
	if s := strings.Join(got, ","); s != "hom22,lib3,fbk40,edt5" {
		t.Fatalf("codes: %s", s)
	}
}

// Similar requests: shared words, best first, common words and "list" ignored, Han
// characters one by one (here: "sort by size", "list by size"); nothing in common is left out.
func TestSimilar(t *testing.T) {
	rs := []Request{
		{Number: 1, Title: "Sort the Mod list by file size", Plus: 2},
		{Number: 2, Title: "Export the Mod list as a Steam collection", Plus: 9},
		{Number: 3, Title: "Sorting by size on disk", Plus: 5},
		{Number: 4, Title: "\u6309\u5927\u5c0f\u6392\u5e8f", Plus: 1},
	}
	got := Similar("sort mods by size", rs, 4)
	if len(got) != 2 || got[0].Number != 1 || got[1].Number != 3 {
		t.Fatalf("similar: %+v", got)
	}
	if got := Similar("\u5217\u8868\u6309\u5927\u5c0f", rs, 4); len(got) != 1 || got[0].Number != 4 {
		t.Fatalf("Han: %+v", got)
	}
	if Similar("the and", rs, 4) != nil {
		t.Fatal("only common words: none")
	}
}

// SendAll: one report goes as before; several in one request with one contact; a relay from
// before split sends answers "empty", which says one at a time.
func TestSendAll(t *testing.T) {
	var bodies []string
	old := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(buf))
		if old && strings.Contains(string(buf), `"reports"`) {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"empty"}`)
			return
		}
		if strings.Contains(string(buf), `"reports"`) {
			fmt.Fprint(w, `{"number":5,"url":"u5","token":"a","issues":[{"number":5,"url":"u5","token":"a"},{"number":6,"url":"u6","token":"b"}]}`)
			return
		}
		fmt.Fprint(w, `{"number":4,"url":"u4","token":"t"}`)
	}))
	defer srv.Close()
	ctx := context.Background()
	s, err := SendAll(ctx, srv.URL+"/report", "RimArcho/test", []Report{{Title: "a", Body: "x", Kind: KindBug, Priority: true}}, "me")
	if err != nil || len(s) != 1 || s[0].Number != 4 || s[0].Token != "t" || !strings.Contains(bodies[0], `"contact":"me"`) || !strings.Contains(bodies[0], `"priority":true`) {
		t.Fatalf("one: %+v %v %s", s, err, bodies[0])
	}
	s, err = SendAll(ctx, srv.URL+"/report", "RimArcho/test", []Report{{Title: "a", Body: "x"}, {Title: "b", Body: "y", Kind: KindFeature}}, "me")
	if err != nil || len(s) != 2 || s[1].Number != 6 || s[1].Token != "b" || strings.Count(bodies[1], `"contact"`) != 1 {
		t.Fatalf("two: %+v %v %s", s, err, bodies[1])
	}
	old = true
	_, err = SendAll(ctx, srv.URL+"/report", "RimArcho/test", []Report{{Title: "a", Body: "x"}, {Title: "b", Body: "y"}}, "")
	var re *RelayError
	if !errors.As(err, &re) || re.Code != "one_at_a_time" {
		t.Fatalf("an old relay: %v", err)
	}
}
