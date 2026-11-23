package modchanges

import (
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// ModRow is one mod's row from the given events and snapshots, as Build makes it (the
// locked versions included), for the tests of each branch of the row's wording.
func ModRow(a *app.App, st *app.LoadoutState, noFiles bool, id string, evs []*store.Event, info map[int64]*store.SnapInfo) (Row, string) {
	b := &changeBuilder{a: a, st: st, mm: st.GameMM(), noFiles: noFiles, tx: English(time.UTC)}
	b.locks = map[string]*app.Locked{}
	for _, l := range st.Locked() {
		if cur := b.locks[l.Upstream.ID]; cur == nil || st.List.Index(l.Copy.ID) >= 0 {
			b.locks[l.Upstream.ID] = l
		}
	}
	return b.mod(id, evs, info)
}

// HomeText is homeText for the tests.
func HomeText(count, launch int) string { return homeText(English(time.UTC), count, launch) }
