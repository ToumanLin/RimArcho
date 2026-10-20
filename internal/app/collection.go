package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// Steam Workshop collections (10 §4.3): read the items through the public, keyless API and
// compare them with what is installed. The network is used only when the player asks.

// SteamCollectionAPI is the public endpoint; tests point App.SteamCollectionAPI elsewhere.
const SteamCollectionAPI = "https://api.steampowered.com/ISteamRemoteStorage/GetCollectionDetails/v1/"

// CollectionItem is one entry of a collection.
type CollectionItem struct {
	WorkshopID string
	Title      string     // from the Workshop ("" if Steam did not say)
	Mod        *store.Mod // the installed instance (nil if not installed)
	Sub        bool       // a nested collection (listed, not expanded)
}

// Collection is a Workshop collection compared with the installed mods.
type Collection struct {
	ID, Title string
	Items     []CollectionItem
}

var collectionIDRe = regexp.MustCompile(`(?:[?&]id=|^)(\d{6,})\b`)

// CollectionID takes a collection link or ID.
func CollectionID(ref string) (string, error) {
	if m := collectionIDRe.FindStringSubmatch(strings.TrimSpace(ref)); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("%q is not a Workshop collection link or ID", ref)
}

func (a *App) steamPost(api string, form url.Values, v any) error {
	req, err := http.NewRequest("POST", api, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := a.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("Steam unreachable: %v", shortErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Steam answered %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(v); err != nil {
		return fmt.Errorf("unexpected answer from Steam: %v", err)
	}
	return nil
}

// workshopTitles fetches item titles, 100 per request.
func (a *App) workshopTitles(ids []string) (map[string]string, error) {
	api := a.SteamAPI
	if api == "" {
		api = SteamAPI
	}
	out := map[string]string{}
	for i := 0; i < len(ids); i += 100 {
		chunk := ids[i:min(i+100, len(ids))]
		form := url.Values{"itemcount": {strconv.Itoa(len(chunk))}}
		for k, id := range chunk {
			form.Set(fmt.Sprintf("publishedfileids[%d]", k), id)
		}
		var r struct {
			Response struct {
				Details []struct {
					ID    string `json:"publishedfileid"`
					Title string `json:"title"`
				} `json:"publishedfiledetails"`
			} `json:"response"`
		}
		if err := a.steamPost(api, form, &r); err != nil {
			return out, err
		}
		for _, d := range r.Response.Details {
			if d.Title != "" {
				out[d.ID] = d.Title
			}
		}
	}
	return out, nil
}

// ReadCollection reads a collection and matches its items with the installed mods.
func (a *App) ReadCollection(ref string) (*Collection, error) {
	id, err := CollectionID(ref)
	if err != nil {
		return nil, err
	}
	st, err := a.Loadout()
	if err != nil {
		return nil, err
	}
	api := a.SteamCollectionAPI
	if api == "" {
		api = SteamCollectionAPI
	}
	var r struct {
		Response struct {
			Details []struct {
				ID       string `json:"publishedfileid"`
				Result   int    `json:"result"`
				Children []struct {
					ID        string `json:"publishedfileid"`
					SortOrder int    `json:"sortorder"`
					FileType  int    `json:"filetype"`
				} `json:"children"`
			} `json:"collectiondetails"`
		} `json:"response"`
	}
	if err := a.steamPost(api, url.Values{"collectioncount": {"1"}, "publishedfileids[0]": {id}}, &r); err != nil {
		return nil, err
	}
	if len(r.Response.Details) == 0 || r.Response.Details[0].Result != 1 || r.Response.Details[0].Children == nil {
		return nil, fmt.Errorf("ws:%s is not a Workshop collection (or it is private)", id)
	}
	kids := r.Response.Details[0].Children
	sort.SliceStable(kids, func(i, j int) bool { return kids[i].SortOrder < kids[j].SortOrder })
	c := &Collection{ID: id}
	ids := []string{id}
	installed := map[string]*store.Mod{}
	for _, m := range st.Ix.Mods {
		w := m.WorkshopID()
		if w == "" || m.Status != identity.Live {
			continue
		}
		if prev := installed[w]; prev == nil || (whyNotLoadable(st, m) == "" && whyNotLoadable(st, prev) != "") {
			installed[w] = m
		}
	}
	for _, k := range kids {
		c.Items = append(c.Items, CollectionItem{WorkshopID: k.ID, Sub: k.FileType == 2, Mod: installed[k.ID]})
		ids = append(ids, k.ID)
	}
	titles, err := a.workshopTitles(ids)
	if err != nil && len(titles) == 0 {
		titles = map[string]string{} // names are a convenience; the comparison stands
	}
	c.Title = titles[id]
	for i := range c.Items {
		c.Items[i].Title = titles[c.Items[i].WorkshopID]
	}
	return c, nil
}

// ImportCollection replaces the workspace with the collection's installed mods in its
// order (one undoable step); items that are not installed are reported, not added.
func (a *App) ImportCollection(c *Collection) (*EditResult, *ImportedList, error) {
	il := newList("collection")
	il.Title = c.Title
	for _, it := range c.Items {
		if !it.Sub {
			il.WorkshopOnly = append(il.WorkshopOnly, it.WorkshopID)
		}
	}
	if len(il.WorkshopOnly) == 0 {
		return nil, il, errors.New("the collection has no mods")
	}
	r, err := a.importList(il, "collection ws:"+c.ID)
	return r, il, err
}
