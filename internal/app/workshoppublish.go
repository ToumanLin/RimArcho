package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToumanLin/RimArcho/internal/steamworks"
)

// WorkshopPublish is the maintainer-only upload, independent of Mod subscriptions.
// It reuses the subscription worker's runtime and never starts the game.
func (a *App) WorkshopPublish(ctx context.Context, request steamworks.PublishRequest, progress func(string)) (id uint64, legal bool, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := context.WithTimeout(ctx, 31*time.Minute)
	defer stop()
	dir, err := a.SteamRuntime()
	if err != nil {
		return 0, false, err
	}
	f, err := os.CreateTemp(filepath.Dir(request.Content), ".workshop-publish-*.json")
	if err != nil {
		return 0, false, err
	}
	defer os.Remove(f.Name())
	writeErr := json.NewEncoder(f).Encode(request)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return 0, false, err
	}
	id = request.ID
	var workerError string
	published, done := false, false
	last := ""
	consume := func(line string) {
		switch {
		case strings.HasPrefix(line, "ERROR "):
			workerError = strings.TrimPrefix(line, "ERROR ")
		case strings.HasPrefix(line, "CREATED "), strings.HasPrefix(line, "PUBLISHED "):
			var kind string
			var got uint64
			var flag int
			if n, _ := fmt.Sscanf(line, "%s %d %d", &kind, &got, &flag); n == 3 && got != 0 {
				id, legal = got, legal || flag != 0
				published = published || kind == "PUBLISHED"
				if progress != nil && kind == "CREATED" {
					progress(fmt.Sprintf("progress: created ws:%d", id))
				}
			}
		case strings.HasPrefix(line, "LEGAL "):
			if progress != nil {
				progress(strings.TrimPrefix(line, "LEGAL "))
			}
		case strings.HasPrefix(line, "UPLOAD "):
			var status uint32
			var got, total uint64
			if n, _ := fmt.Sscanf(line, "UPLOAD %d %d %d", &status, &got, &total); n != 3 {
				return
			}
			state := map[uint32]string{0: "waiting", 1: "preparing configuration", 2: "preparing content", 3: "uploading content", 4: "uploading preview", 5: "committing changes"}[status]
			text := "progress: Workshop " + state
			if total > 0 {
				text += fmt.Sprintf(" %d%% (%d / %d bytes)", min(uint64(float64(got)*100/float64(total)), 100), got, total)
			}
			key := fmt.Sprintf("%d/%d", status, uint64(float64(got)*10/float64(max(total, 1))))
			if progress != nil && key != last {
				progress(text)
				last = key
			}
		case line == steamworks.DoneLine(steamworks.Publish, 1):
			done = true
		}
	}
	args := []string{steamworks.Publish, f.Name()}
	switch {
	case a.SteamWorkerStream != nil:
		err = a.SteamWorkerStream(ctx, dir, args, consume)
	case a.SteamWorker != nil:
		var out string
		out, err = a.SteamWorker(dir, args)
		for _, line := range strings.Split(out, "\n") {
			consume(strings.TrimSpace(line))
		}
	default:
		err = steamWorkerStreaming(ctx, dir, args, consume)
	}
	if workerError != "" {
		err = errors.New(workerError)
	}
	if ctx.Err() != nil {
		err = fmt.Errorf("Workshop publish interrupted: %w; check the item page before retrying", ctx.Err())
	}
	if err == nil && (!published || !done) {
		err = errors.New("Steam worker exited without confirming publication; check the item page before retrying")
	}
	if err != nil && id != 0 {
		err = fmt.Errorf("%w; item: https://steamcommunity.com/sharedfiles/filedetails/?id=%d", err, id)
		if legal {
			err = fmt.Errorf("%w; accept the Steam Workshop legal agreement on that page; the item stays hidden until accepted", err)
		}
	}
	return id, legal, err
}
