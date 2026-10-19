// Spike (TUI v2 info round): where do mod descriptions live? Reads About.xml of every
// workshop mod (read-only) and the Steam Workshop description of the same items through the
// public, keyless ISteamRemoteStorage/GetPublishedFileDetails, then measures both.
//
//	go run ./spikes/infodesc -root <workshop/content/294100> -cache <file.json> [-fetch]
//
// Without -fetch it only reads the cache written by an earlier run.
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type about struct {
	Name        string `xml:"name"`
	Description string `xml:"description"`
	ByVersion   struct {
		Items []struct {
			XMLName xml.Name
			Text    string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"descriptionsByVersion"`
}

type detail struct {
	ID          string `json:"publishedfileid"`
	Result      int    `json:"result"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TimeUpdated int64  `json:"time_updated"`
	PreviewURL  string `json:"preview_url"`
}

var (
	imgRe   = regexp.MustCompile(`(?is)\[img\](.*?)\[/img\]`)
	tagRe   = regexp.MustCompile(`(?s)\[/?[a-zA-Z0-9*]+(=[^\]]*)?\]`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// plain strips BBCode and images, collapsing white space.
func plain(s string) string {
	s = imgRe.ReplaceAllString(s, " ")
	s = tagRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

func fetch(ids []string) ([]detail, error) {
	var out []detail
	for i := 0; i < len(ids); i += 100 {
		end := min(i+100, len(ids))
		form := url.Values{"itemcount": {fmt.Sprint(end - i)}}
		for k, id := range ids[i:end] {
			form.Set(fmt.Sprintf("publishedfileids[%d]", k), id)
		}
		resp, err := http.PostForm("https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/", form)
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		var r struct {
			Response struct {
				Details []detail `json:"publishedfiledetails"`
			} `json:"response"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("batch %d: %v: %.200s", i, err, b)
		}
		out = append(out, r.Response.Details...)
		time.Sleep(500 * time.Millisecond) // gentle
	}
	return out, nil
}

func bucket(n int) string {
	switch {
	case n == 0:
		return "empty"
	case n < 80:
		return "1–79"
	case n < 300:
		return "80–299"
	case n < 1000:
		return "300–999"
	}
	return "≥1000"
}

func main() {
	root := flag.String("root", `D:\SteamLibrary\steamapps\workshop\content\294100`, "workshop content dir")
	cache := flag.String("cache", "", "JSON cache of the Workshop details")
	doFetch := flag.Bool("fetch", false, "query Steam (else read -cache)")
	flag.Parse()
	ents, err := os.ReadDir(*root)
	if err != nil {
		panic(err)
	}
	abouts := map[string]about{}
	var ids []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		var b []byte
		for _, fn := range []string{"About.xml", "About.xml.disabled"} {
			if b, err = os.ReadFile(filepath.Join(*root, e.Name(), "About", fn)); err == nil {
				break
			}
		}
		if b == nil {
			continue // residue without About.xml
		}
		var a about
		xml.Unmarshal(b, &a)
		abouts[e.Name()] = a
		ids = append(ids, e.Name())
	}
	sort.Strings(ids)
	var ds []detail
	if *doFetch {
		if ds, err = fetch(ids); err != nil {
			panic(err)
		}
		b, _ := json.Marshal(ds)
		os.WriteFile(*cache, b, 0o644)
	} else {
		b, err := os.ReadFile(*cache)
		if err != nil {
			panic(err)
		}
		json.Unmarshal(b, &ds)
	}
	ws := map[string]detail{}
	for _, d := range ds {
		ws[d.ID] = d
	}
	type row struct{ about, ws, wsRaw, imgs int }
	var rows []row
	aboutB, wsB := map[string]int{}, map[string]int{}
	byVer, ok, imgOnly, imgAny, aboutShortWsRich, bothRich, neither, imgHosts := 0, 0, 0, 0, 0, 0, 0, map[string]int{}
	var imgTotal, rawTotal int
	for _, id := range ids {
		a := abouts[id]
		al := len([]rune(plain(a.Description)))
		if len(a.ByVersion.Items) > 0 {
			byVer++
			for _, it := range a.ByVersion.Items {
				al = max(al, len([]rune(plain(it.Text))))
			}
		}
		d, has := ws[id]
		if !has || d.Result != 1 {
			aboutB[bucket(al)]++
			rows = append(rows, row{al, -1, -1, 0})
			continue
		}
		ok++
		wl := len([]rune(plain(d.Description)))
		im := imgRe.FindAllStringSubmatch(d.Description, -1)
		for _, m := range im {
			if u, err := url.Parse(strings.TrimSpace(m[1])); err == nil {
				imgHosts[u.Host]++
			}
		}
		imgTotal += len(im)
		rawTotal += len(d.Description)
		if len(im) > 0 {
			imgAny++
			if wl < 80 {
				imgOnly++
			}
		}
		aboutB[bucket(al)]++
		wsB[bucket(wl)]++
		switch {
		case al < 80 && wl >= 300:
			aboutShortWsRich++
		case al >= 300 && wl >= 300:
			bothRich++
		case al < 80 && wl < 80 && len(im) == 0:
			neither++
		}
		rows = append(rows, row{al, wl, len(d.Description), len(im)})
	}
	fmt.Printf("workshop folders with About.xml: %d; Workshop details returned: %d (result=1: %d)\n", len(ids), len(ds), ok)
	fmt.Printf("About.xml with descriptionsByVersion: %d\n", byVer)
	order := []string{"empty", "1–79", "80–299", "300–999", "≥1000"}
	fmt.Println("\nplain-text length (characters, BBCode and images stripped):")
	fmt.Printf("  %-10s %8s %10s\n", "bucket", "About", "Workshop")
	for _, b := range order {
		fmt.Printf("  %-10s %8d %10d\n", b, aboutB[b], wsB[b])
	}
	fmt.Printf("\nWorkshop descriptions with [img]: %d of %d (%.0f%%); images in total: %d\n", imgAny, ok, 100*float64(imgAny)/float64(max(ok, 1)), imgTotal)
	fmt.Printf("Workshop description mostly images (< 80 chars of text, ≥ 1 image): %d\n", imgOnly)
	fmt.Printf("About.xml short (< 80) but Workshop rich (≥ 300): %d\n", aboutShortWsRich)
	fmt.Printf("both rich (≥ 300): %d\n", bothRich)
	fmt.Printf("neither (both < 80, no images): %d\n", neither)
	fmt.Printf("mean Workshop description size: %d bytes\n", rawTotal/max(ok, 1))
	type hc struct {
		h string
		n int
	}
	var hs []hc
	for h, n := range imgHosts {
		hs = append(hs, hc{h, n})
	}
	sort.Slice(hs, func(i, j int) bool { return hs[i].n > hs[j].n })
	fmt.Println("\nimage hosts:")
	for i, h := range hs {
		if i == 10 {
			break
		}
		fmt.Printf("  %5d  %s\n", h.n, h.h)
	}
	// Same-text check: About and Workshop are often the same text.
	same := 0
	for _, id := range ids {
		d, has := ws[id]
		if !has || d.Result != 1 {
			continue
		}
		pa, pw := plain(abouts[id].Description), plain(d.Description)
		if len(pa) >= 80 && len(pw) >= 80 && (strings.Contains(pw, pa[:min(len(pa), 60)]) || strings.Contains(pa, pw[:min(len(pw), 60)])) {
			same++
		}
	}
	fmt.Printf("\nAbout and Workshop start with the same text (both ≥ 80): %d\n", same)
	// Image-heavy: at least 3 images and fewer than 100 characters of text per image.
	heavy := 0
	for _, r := range rows {
		if r.imgs >= 3 && r.ws < 100*r.imgs {
			heavy++
		}
	}
	fmt.Printf("image-heavy (≥ 3 images, < 100 text chars per image): %d\n", heavy)
	// BBCode tags in use, and image formats.
	tagN, extN := map[string]int{}, map[string]int{}
	openRe := regexp.MustCompile(`\[([a-zA-Z0-9*]+)(=[^\]]*)?\]`)
	for _, d := range ds {
		for _, m := range openRe.FindAllStringSubmatch(d.Description, -1) {
			tagN[strings.ToLower(m[1])]++
		}
		for _, m := range imgRe.FindAllStringSubmatch(d.Description, -1) {
			u := strings.ToLower(strings.TrimSpace(m[1]))
			if i := strings.IndexAny(u, "?#"); i >= 0 {
				u = u[:i]
			}
			ext := filepath.Ext(u)
			if len(ext) > 6 || ext == "" {
				ext = "(none)"
			}
			extN[ext]++
		}
	}
	type kv struct {
		k string
		n int
	}
	top := func(m map[string]int, n int) string {
		var l []kv
		for k, v := range m {
			l = append(l, kv{k, v})
		}
		sort.Slice(l, func(i, j int) bool { return l[i].n > l[j].n })
		var s []string
		for i, x := range l {
			if i == n {
				break
			}
			s = append(s, fmt.Sprintf("%s %d", x.k, x.n))
		}
		return strings.Join(s, ", ")
	}
	fmt.Printf("BBCode tags: %s\n", top(tagN, 20))
	fmt.Printf("image formats: %s\n", top(extN, 8))
}
