package cmds

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ToumanLin/RimArcho/internal/app"
	"github.com/ToumanLin/RimArcho/internal/render"
)

// Stream receives lines of commands that keep running (log --follow); the CLI points it at
// stdout. When nil the lines are collected into the command's output.
var Stream func(string)

// logFollow follows Player.log (10 §3.4): new clusters as they appear, until the game
// exits (or Ctrl+C when it was not running); --once reads what is there and stops.
func logFollow(c *Ctx, a *Args) (*render.Doc, error) {
	st, err := c.App.Loadout()
	if err != nil {
		return nil, err
	}
	path := a.Str("file")
	if path == "" {
		if path, _ = c.App.LogPaths(st); path == "" {
			return nil, fmt.Errorf("no Player.log found under %s (archo log --follow --file <path>)", st.Ix.Env.UserDataRoot)
		}
	}
	lv := c.App.FollowLog(path, c.App.CompanionLog(st))
	d, root := render.New("following " + path)
	emit := func(s string) {
		if Stream != nil {
			Stream(s)
		} else {
			root.Add(s)
		}
	}
	if Stream != nil {
		Stream("following " + path)
	}
	asJSON := a.Bool("json")
	show := func(cs []*app.LiveCluster) {
		for _, lc := range cs {
			if asJSON {
				b, _ := json.Marshal(liveJSON(st.Ix, lc))
				emit(string(b))
				continue
			}
			line := fmt.Sprintf("%s  %s x%d  %s", lc.At.In(c.App.Loc).Format("15:04:05"), lc.Class, lc.Count, firstMessage(lc))
			if r := lc.Report; r != nil && len(r.Attr) > 0 {
				line += "  → " + modLabel(st.Ix, r.Attr[0].Instance) + " (" + r.Attr[0].Role + ")"
			}
			emit(line)
		}
	}
	if a.Bool("once") {
		cs, err := lv.Poll(c.App)
		if err != nil {
			return nil, err
		}
		end, err := lv.Close(c.App)
		if err != nil {
			return nil, err
		}
		show(append(cs, end...))
		root.KV("clusters", fmt.Sprint(len(lv.Clusters())))
		return d, nil
	}
	run, _ := c.App.GameRunning()
	if run == nil {
		emit("the game is not running; following until Ctrl+C")
	} else {
		emit(fmt.Sprintf("the game is running (pid %d, since %s); following until it exits", run.PID, run.Started.In(c.App.Loc).Format("15:04")))
	}
	wasRunning := run != nil
	for tick := 0; ; tick++ {
		cs, err := lv.Poll(c.App)
		if err != nil {
			return nil, err
		}
		show(cs)
		if tick%3 == 2 && wasRunning {
			if r, _ := c.App.GameRunning(); r == nil {
				end, err := lv.Close(c.App)
				if err != nil {
					return nil, err
				}
				show(end)
				emit(fmt.Sprintf("the game exited: %d clusters this run (archo log for the full analysis)", len(lv.Clusters())))
				return d, nil
			}
		}
		time.Sleep(time.Second)
	}
}

func firstMessage(lc *app.LiveCluster) string {
	msg := ""
	if lc.Report != nil && lc.Report.First != nil {
		msg = lc.Report.First.Message
	}
	if len(msg) > 100 {
		msg = msg[:97] + "..."
	}
	return msg
}

type liveOut struct {
	At       string   `json:"at"`
	Class    string   `json:"class"`
	Count    int      `json:"count"`
	Level    string   `json:"level,omitempty"`
	Message  string   `json:"message"`
	Mods     []string `json:"mods,omitempty"` // "#ref role"
	Template string   `json:"template"`
}

func liveJSON(ix *app.Index, lc *app.LiveCluster) liveOut {
	o := liveOut{At: lc.At.Format(time.RFC3339), Class: lc.Class, Count: lc.Count, Level: lc.Level, Message: firstMessage(lc), Template: lc.Template}
	if lc.Report != nil {
		for _, at := range lc.Report.Attr {
			o.Mods = append(o.Mods, ix.Ref(at.Instance)+" "+at.Role)
		}
	}
	return o
}
