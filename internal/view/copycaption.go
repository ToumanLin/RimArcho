package view

import (
	"encoding/json"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/scan"
	"github.com/ToumanLin/RimArcho/internal/store"
)

// CopyCaption is shared by the editor and the inventory. It never contains a path.
func CopyCaption(a *app.App, st *app.LoadoutState, m *store.Mod) string {
	s := st.Ix.Source(m.SourceID)
	i := &app.LibInstance{Mod: m, Source: a.DirectoryRole(s), DirectoryValues: a.DirectoryValues(s), DirectoryName: app.DirectoryName(s), Created: m.FirstSeen, Subscription: a.SubscriptionOf(m)}
	if i.Subscription != nil && (i.Subscription.Kind == app.LibGit || i.Subscription.Kind == app.LibPage) {
		i.Source = i.Subscription.Kind
	}
	return libLabel(i)
}

func copyCreated(m *store.Mod, fallback time.Time) (bool, time.Time) {
	var origin scan.Origin
	if m.Origin != "" {
		_ = json.Unmarshal([]byte(m.Origin), &origin)
	}
	if t, err := time.Parse(time.RFC3339, origin.CopiedAt); err == nil {
		fallback = t
	}
	return origin.InstanceID != "" || origin.CopiedAt != "", fallback
}
