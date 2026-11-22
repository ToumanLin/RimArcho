package feedback

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Feature requests already on GitHub (16 §2.14 Feedback, "similar requests"): the relay lists
// the open issues labelled as feature requests with their +1 counts, and takes a +1 from a
// player without a GitHub account. RimArcho asks only while a feature request is being written.

// Request is one open feature request.
type Request struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Plus   int    `json:"plus"`
}

// relayBase is the relay's address without /report, or "" when it does not end so.
func relayBase(relay string) string {
	base, ok := strings.CutSuffix(strings.TrimRight(relay, "/"), "/report")
	if !ok {
		return ""
	}
	return base
}

// FetchRequests asks the relay for the open feature requests.
func FetchRequests(ctx context.Context, relay, agent string) ([]Request, error) {
	base := relayBase(relay)
	if err := relayAddress(base + "/requests"); base == "" || err != nil {
		return nil, &RelayError{Code: "not_configured"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/requests", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return nil, &RelayError{Status: res.StatusCode, Code: http.StatusText(res.StatusCode)}
	}
	var out struct {
		Requests []Request `json:"requests"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out.Requests, nil
}

// Plus adds the player's +1 to request n through the relay and returns its new count.
func Plus(ctx context.Context, relay, agent string, n int) (int, error) {
	base := relayBase(relay)
	if base == "" || n <= 0 {
		return 0, &RelayError{Code: "not_configured"}
	}
	var out struct {
		Plus  int    `json:"plus"`
		Error string `json:"error"`
	}
	status, err := post(ctx, base+"/plus/"+strconv.Itoa(n), agent, struct{}{}, &out)
	if err != nil {
		return 0, err
	}
	if status != http.StatusOK {
		code := out.Error
		if code == "" {
			code = http.StatusText(status)
		}
		return 0, &RelayError{Status: status, Code: code}
	}
	return out.Plus, nil
}

// Similar is the requests most like title, best first, at most max: the share of the title's
// words (three letters or more, or any Han character) found in the request's title. Requests
// with none in common are left out.
func Similar(title string, rs []Request, max int) []Request {
	want := words(title)
	if len(want) == 0 {
		return nil
	}
	type scored struct {
		r Request
		s float64
	}
	var out []scored
	for _, r := range rs {
		have := words(r.Title)
		hit := 0
		for w := range want {
			if have[w] {
				hit++
			}
		}
		if hit > 0 {
			out = append(out, scored{r, float64(hit) / float64(len(want))})
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].s > out[j-1].s || out[j].s == out[j-1].s && out[j].r.Plus > out[j-1].r.Plus); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	var rs2 []Request
	for i := 0; i < len(out) && i < max; i++ {
		rs2 = append(rs2, out[i].r)
	}
	return rs2
}

// stop are English words and Han characters (the, of, is, I, can ...) too common to tell requests apart.
var stop = map[string]bool{"the": true, "and": true, "for": true, "with": true, "from": true, "that": true, "this": true, "mod": true, "mods": true, "list": true, "lists": true, "can": true, "add": true, "when": true, "into": true, "should": true, "would": true, "like": true, "want": true,
	"\u7684": true, "\u4e86": true, "\u662f": true, "\u5728": true, "\u4e00": true, "\u4e2a": true, "\u6211": true, "\u80fd": true, "\u4e0d": true, "\u6709": true, "\u548c": true, "\u5417": true, "\u628a": true, "\u8981": true, "\u53ef": true, "\u4ee5": true}

// words are the lower-case words of s worth comparing: Latin words of three letters or more
// (a plural "s" dropped) and each Han character on its own.
func words(s string) map[string]bool {
	out := map[string]bool{}
	var cur []rune
	flush := func() {
		w := string(cur)
		cur = cur[:0]
		if len([]rune(w)) < 3 || stop[w] {
			return
		}
		if len(w) > 3 && strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") {
			w = w[:len(w)-1]
		}
		out[w] = true
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			if !stop[string(r)] {
				out[string(r)] = true
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

// PlusFile records the requests this computer gave a +1 (RimArcho's data directory, 02 §1),
// so the page shows them as given and does not send a second.
const PlusFile = "feedback-plus.json"

// LoadPlus reads the requests given a +1; a missing or broken file is none.
func LoadPlus(dir string) map[int]bool {
	out := map[int]bool{}
	b, err := os.ReadFile(filepath.Join(dir, PlusFile))
	if err != nil {
		return out
	}
	var ns []int
	if json.Unmarshal(b, &ns) == nil {
		for _, n := range ns {
			out[n] = true
		}
	}
	return out
}

// SavePlus adds n to the record.
func SavePlus(dir string, n int) error {
	have := LoadPlus(dir)
	if have[n] {
		return nil
	}
	ns := []int{n}
	for k := range have {
		ns = append(ns, k)
	}
	sort.Ints(ns)
	b, _ := json.Marshal(ns)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(dir, PlusFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, PlusFile))
}
