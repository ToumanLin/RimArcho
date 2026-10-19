package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // first frame (06 §6.4 item 6)
	_ "image/jpeg"
	"image/png"
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

// Descriptions (06 §6.4): About.xml from disk, the Steam Workshop description fetched on
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
	// Show: which to display (06 §6.4 item 2): "workshop", "about", or "both".
	Show string
}

type workshopCache struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TimeUpdated int64  `json:"time_updated"`
	Fetched     int64  `json:"fetched"`
	Result      int    `json:"result"`
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

// choose implements 06 §6.4 item 2.
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
	api := a.SteamAPI
	if api == "" {
		api = SteamAPI
	}
	form := url.Values{"itemcount": {"1"}, "publishedfileids[0]": {id}}
	req, err := http.NewRequest("POST", api, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("Steam unreachable: %v", shortErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Steam answered %s", resp.Status)
	}
	var r struct {
		Response struct {
			Details []struct {
				ID          string `json:"publishedfileid"`
				Result      int    `json:"result"`
				Title       string `json:"title"`
				Description string `json:"description"`
				TimeUpdated int64  `json:"time_updated"`
			} `json:"publishedfiledetails"`
		} `json:"response"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("unexpected answer from Steam: %v", err)
	}
	if len(r.Response.Details) == 0 {
		return nil, errors.New("Steam returned no details")
	}
	x := r.Response.Details[0]
	return &workshopCache{ID: id, Title: x.Title, Description: x.Description, TimeUpdated: x.TimeUpdated, Fetched: a.Now().Unix(), Result: x.Result}, nil
}

func shortErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// DescImageMaxWidth is the width images are scaled to in the cache (06 §6.4 item 6).
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
	if f, err := os.Open(path); err == nil {
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err == nil {
			return path, cfg.Width, cfg.Height, nil
		}
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
	img, _, err := image.Decode(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", 0, 0, fmt.Errorf("cannot decode the image: %v", err)
	}
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 || b.Dx()*b.Dy() > 40e6 {
		return "", 0, 0, errors.New("image size out of range")
	}
	tw := min(b.Dx(), DescImageMaxWidth)
	th := max(b.Dy()*tw/b.Dx(), 1)
	out := ScaleBox(img, tw, th)
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, out); err != nil {
		return "", 0, 0, err
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return "", 0, 0, err
	}
	return path, tw, th, nil
}

// ScaleBox scales with a box filter (averaging the source pixels under each target pixel).
func ScaleBox(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	rgba, ok := src.(*image.RGBA)
	if !ok {
		rgba = image.NewRGBA(b)
		draw.Draw(rgba, b, src, b.Min, draw.Src)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*b.Dy()/h, (y+1)*b.Dy()/h
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < w; x++ {
			x0, x1 := x*b.Dx()/w, (x+1)*b.Dx()/w
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, al, n uint32
			for yy := y0; yy < y1; yy++ {
				i := rgba.PixOffset(b.Min.X+x0, b.Min.Y+yy)
				for xx := x0; xx < x1; xx++ {
					r, g, bl, al, n = r+uint32(rgba.Pix[i]), g+uint32(rgba.Pix[i+1]), bl+uint32(rgba.Pix[i+2]), al+uint32(rgba.Pix[i+3]), n+1
					i += 4
				}
			}
			j := dst.PixOffset(x, y)
			dst.Pix[j], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = uint8(r/n), uint8(g/n), uint8(bl/n), uint8(al/n)
		}
	}
	return dst
}
