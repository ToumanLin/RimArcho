package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	_ "image/gif" // first frame (16 §4.3 item 6)
	_ "image/jpeg"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/bbcode"
	"github.com/ToumanLin/RimArcho/internal/env"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// UserAgent is sent with Workshop and image requests.
const UserAgent = "RimArcho (+https://github.com/ToumanLin/RimArcho)"

// Descriptions (16 §4.3): About.xml from disk, the Steam Workshop description fetched on
// demand for one mod and cached, and the images it embeds.

// SteamAPI is the public, keyless endpoint; tests point App.SteamAPI elsewhere.
const SteamAPI = "https://api.steampowered.com/ISteamRemoteStorage/GetPublishedFileDetails/v1/"

// Description of one mod.
type Description struct {
	About    string // About.xml (the game version's descriptionsByVersion entry if any)
	Workshop string // BBCode from the Workshop page; "" when unknown
	// WorkshopState says why there is no Workshop text, or where it came from:
	// "" (not a workshop mod), "fetched", "cached", "offline" (workshop.fetch off),
	// "not cached" (fetch false), or an error message.
	WorkshopState string
	Updated       time.Time // Workshop time_updated
	// Show: which to display (16 §4.3 item 2): "workshop", "about", or "both".
	Show string
}

type workshopCache struct {
	Tags        []string `json:"tags,omitempty"`
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	TimeUpdated int64    `json:"time_updated"`
	Fetched     int64    `json:"fetched"`
	Result      int      `json:"result"`
}

func (a *App) httpClient() *http.Client {
	if a.HTTP != nil {
		return a.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// WorkshopFetch reports whether RimArcho may go online for descriptions and images.
func (a *App) WorkshopFetch() bool { return a.Store.Meta("cfg:workshop.fetch") != "off" }
func (a *App) WorkshopImages() bool {
	return a.WorkshopFetch() && a.Store.Meta("cfg:workshop.images") != "off"
}

// ModPath is the mod's folder on disk.
func (a *App) ModPath(ix *Index, m *store.Mod) string {
	if s := ix.Source(m.SourceID); s != nil {
		return filepath.Join(s.Root, filepath.FromSlash(m.Rel))
	}
	return m.RealPath
}

// AboutDescription reads <description> (or the game version's descriptionsByVersion
// entry) from the mod's About.xml (or About.xml.disabled). Read-only.
func (a *App) AboutDescription(ix *Index, m *store.Mod) string {
	dir := filepath.Join(a.ModPath(ix, m), "About")
	var b []byte
	for _, fn := range []string{"About.xml", "About.xml.disabled"} {
		var err error
		if b, err = os.ReadFile(filepath.Join(dir, fn)); err == nil {
			break
		}
	}
	if b == nil {
		return ""
	}
	var x struct {
		Description string `xml:"description"`
		ByVersion   struct {
			Items []struct {
				XMLName xml.Name
				Text    string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"descriptionsByVersion"`
	}
	dec := xml.NewDecoder(bytes.NewReader(b))
	dec.Strict = false
	dec.Decode(&x)
	if mm := env.MajorMinor(ix.Env.GameVersion); mm != "" {
		for _, it := range x.ByVersion.Items {
			if strings.EqualFold(it.XMLName.Local, "v"+mm) && strings.TrimSpace(it.Text) != "" {
				return strings.TrimSpace(it.Text)
			}
		}
	}
	return strings.TrimSpace(x.Description)
}

func (a *App) cacheDir(sub string) string { return filepath.Join(a.Home, "cache", sub) }

// Describe returns the mod's description. With fetch, a missing or outdated Workshop text
// is fetched (unless workshop.fetch is off); without, only the cache is read.
func (a *App) Describe(ix *Index, m *store.Mod, fetch, refresh bool) *Description {
	d := &Description{About: a.AboutDescription(ix, m), Show: "about"}
	id := m.WorkshopID()
	if id == "" || !isDigits(id) {
		d.WorkshopState = ""
		return d
	}
	path := filepath.Join(a.cacheDir("workshop"), id+".json")
	var c workshopCache
	cached := false
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &c) == nil {
		cached = true
	}
	fresh := cached && !refresh
	if fresh {
		switch {
		case m.ACFTimeUpdated > 0:
			fresh = c.TimeUpdated >= m.ACFTimeUpdated
		default:
			fresh = a.Now().Sub(time.Unix(c.Fetched, 0)) < 7*24*time.Hour
		}
	}
	switch {
	case fresh:
		d.WorkshopState = "cached"
	case !a.WorkshopFetch():
		d.WorkshopState = "offline"
	case !fetch:
		d.WorkshopState = "not cached"
	default:
		nc, err := a.fetchWorkshop(id)
		if err != nil {
			d.WorkshopState = err.Error()
			break
		}
		c, cached = *nc, true
		d.WorkshopState = "fetched"
		if b, err := json.Marshal(c); err == nil {
			os.MkdirAll(filepath.Dir(path), 0o755)
			os.WriteFile(path, b, 0o644)
		}
	}
	if cached && c.Result == 1 {
		d.Workshop, d.Updated = c.Description, time.Unix(c.TimeUpdated, 0)
		if d.WorkshopState != "fetched" && d.WorkshopState != "cached" {
			d.WorkshopState = "cached" // outdated, but better than nothing while offline
		}
	}
	d.Show = choose(d.About, d.Workshop)
	return d
}

// choose implements 16 §4.3 item 2.
func choose(about, ws string) string {
	if strings.TrimSpace(ws) == "" {
		return "about"
	}
	ab, wb := bbcode.Parse(about), bbcode.Parse(ws)
	if bbcode.TextLen(ab) < 300 {
		return "workshop"
	}
	pa, pw := startText(ab), startText(wb)
	n := min(60, len(pa), len(pw))
	if n > 0 && pa[:n] == pw[:n] {
		return "workshop"
	}
	return "both"
}

// startText is the text of a description without its images, spaces collapsed: what
// choose compares (Workshop pages often open with a banner image).
func startText(bs []bbcode.Block) string {
	var keep []bbcode.Block
	for _, b := range bs {
		if b.Kind != bbcode.Image {
			keep = append(keep, b)
		}
	}
	return strings.Join(strings.Fields(bbcode.Plain(keep)), " ")
}

// imageURL is the address to download: imgur page-style links to an image file
// (imgur.com/x.png) go straight to i.imgur.com instead of through a redirect.
func imageURL(u *url.URL) string {
	if (u.Host == "imgur.com" || u.Host == "www.imgur.com") && path.Ext(u.Path) != "" {
		c := *u
		c.Host = "i.imgur.com"
		return c.String()
	}
	return u.String()
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func (a *App) fetchWorkshop(id string) (*workshopCache, error) {
	got, err := a.postWorkshopDetails(a.subscriptionContext(), []string{id})
	if err != nil {
		return nil, err
	}
	d, ok := got[id]
	if !ok {
		return nil, errors.New("Steam returned no details")
	}
	return d, nil
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// DescImageMaxWidth is the width images are scaled to in the cache (16 §4.3 item 6).
const DescImageMaxWidth = 720

// DescImage returns a description image as a PNG file in the cache, downloading,
// decoding and scaling it on first use. Only http(s) URLs; at most 16 MB downloaded.
func (a *App) DescImage(rawURL string) (path string, w, h int, err error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", 0, 0, errors.New("not an http(s) image")
	}
	sum := sha1.Sum([]byte(u.String()))
	path = filepath.Join(a.cacheDir("workshop-img"), hex.EncodeToString(sum[:10])+".png")
	if w, h, ok := cachedPNG(path); ok {
		return path, w, h, nil
	}
	if !a.WorkshopImages() {
		return "", 0, 0, errors.New("image downloads are off (workshop.images)")
	}
	req, err := http.NewRequest("GET", imageURL(u), nil)
	if err != nil {
		return "", 0, 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return "", 0, 0, fmt.Errorf("download failed: %v", shortErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", 0, 0, fmt.Errorf("download failed: %s", resp.Status)
	}
	// Download whole, then wait for a decode slot: a slow server holds no slot.
	body, err := io.ReadAll(io.LimitReader(resp.Body, descImageMaxBytes+1))
	if err != nil {
		return "", 0, 0, fmt.Errorf("download failed: %v", shortErr(err))
	}
	if len(body) > descImageMaxBytes {
		return "", 0, 0, errors.New("image larger than 16 MB")
	}
	w, h, err = makeCachedPNG(path, DescImageMaxWidth, func() (io.ReadSeeker, func(), error) {
		return bytes.NewReader(body), func() {}, nil
	})
	var bad decodeError
	switch {
	case errors.Is(err, errImageSize):
		return "", 0, 0, errors.New("image size out of range")
	case errors.As(err, &bad):
		return "", 0, 0, fmt.Errorf("cannot decode the image: %v", bad.error)
	case err != nil:
		return "", 0, 0, err
	}
	return path, w, h, nil
}

// descImageMaxBytes is the most a description image may download.
const descImageMaxBytes = 16 << 20
