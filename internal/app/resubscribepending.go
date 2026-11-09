package app

import (
	"encoding/json"
	"sort"

	"github.com/ToumanLin/RimArcho/internal/identity"
	"github.com/ToumanLin/RimArcho/internal/source"
)

// resubscribePendingKey records the Workshop items a resubscribe has unsubscribed but not yet
// subscribed again (JSON: Workshop ID → name). It is written before the unsubscribe and
// cleared as Steam confirms each subscription, so a crash, a lost Steam connection or a
// failed folder cleanup between the two steps is offered again in Can it run (16 §2.9).
const resubscribePendingKey = "workshop:resubscribe-pending"

// PendingResubscribe is an item an interrupted resubscribe left unsubscribed.
type PendingResubscribe struct {
	WorkshopID string `json:"workshopId"`
	Name       string `json:"name"`
}

func (a *App) readResubscribePending() map[string]string {
	m := map[string]string{}
	if v := a.Store.Meta(resubscribePendingKey); v != "" {
		json.Unmarshal([]byte(v), &m)
	}
	return m
}

func (a *App) writeResubscribePending(m map[string]string) error {
	if len(m) == 0 {
		_, err := a.Store.DB.Exec(`DELETE FROM meta WHERE key=?`, resubscribePendingKey)
		return err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return a.Store.SetMeta(resubscribePendingKey, string(b))
}

// markResubscribePending adds items (Workshop ID → name) before they are unsubscribed.
func (a *App) markResubscribePending(items map[string]string) error {
	m := a.readResubscribePending()
	for id, name := range items {
		m[id] = name
	}
	return a.writeResubscribePending(m)
}

// settleResubscribePending removes items that need no subscribing again: Steam confirmed
// their subscription, or it did not unsubscribe them.
func (a *App) settleResubscribePending(ids []string) {
	m := a.readResubscribePending()
	n := len(m)
	for _, id := range ids {
		delete(m, id)
	}
	if len(m) != n {
		a.writeResubscribePending(m)
	}
}

// ResubscribePending lists the items an interrupted resubscribe left unsubscribed, leaving
// out those subscribed again since in another way (in the Steam client): a live,
// non-residue copy in the Workshop folder.
func (a *App) ResubscribePending(st *LoadoutState) []PendingResubscribe {
	m := a.readResubscribePending()
	if len(m) == 0 {
		return nil
	}
	if st != nil && st.Ix != nil {
		for _, mod := range st.Ix.Mods {
			if mod.Kind == source.Workshop && mod.Status == identity.Live && !mod.Residue {
				delete(m, mod.WorkshopID())
			}
		}
	}
	out := make([]PendingResubscribe, 0, len(m))
	for id, name := range m {
		out = append(out, PendingResubscribe{WorkshopID: id, Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
