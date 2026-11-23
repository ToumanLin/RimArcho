package view

import (
	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/corpus"
)

// CorpusChip is Can it run's shared-log data source (05 §4.6): a small week pill, its source
// and log count in the tooltip (visuals first, 16 §2.4).
type CorpusChip struct {
	Week  string `json:"week,omitempty"` // W47; empty when this game version has none
	Game  string `json:"game"`
	Tip   string `json:"tip"`
	Newer bool   `json:"newer,omitempty"` // a bundle needs a newer RimArcho
}

func corpusChip(a *app.App) *CorpusChip {
	ch := a.Corpus()
	c := &CorpusChip{Week: corpus.ShortWeek(ch.Data), Game: ch.Game, Newer: ch.NewerFormat}
	if ch.Data == "" {
		c.Week = ""
		c.Tip = Tf("no shared-log data for %s yet", ch.Game)
	} else {
		from := T("built in")
		switch ch.Origin {
		case corpus.Monitor:
			from = Tf("Archo Monitor folder %s", ch.Label)
		case corpus.Workshop:
			from = T("Workshop Archo Monitor")
		}
		c.Tip = Tf("shared-log data %s, %s; %s, %d logs", c.Week, ch.Game, from, ch.Logs)
	}
	if ch.NewerFormat {
		c.Tip += "; " + T("newer corpus data needs a newer RimArcho")
	}
	return c
}
