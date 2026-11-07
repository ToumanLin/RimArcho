package gui

import (
	"encoding/json"
	"testing"

	"github.com/ToumanLin/RimArcho/internal/testkit"
)

func TestArchoMotionStartupAndSettings(t *testing.T) {
	t.Parallel()
	e := testkit.New(t)
	api := NewAPI(e.App, nil)
	t.Cleanup(api.Close)
	if got := api.Init().ArchoMotion; got != "live" {
		t.Fatalf("default motion: %q", got)
	}
	for _, value := range []string{"off", "still", "live"} {
		if _, err := api.ApplySetting([]string{"config", "set", "ui.archo.motion", "{}"}, value); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(api.Init())
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["archoMotion"] != value {
			t.Fatalf("saved motion %q, payload: %s", value, data)
		}
	}
	if _, err := api.ApplySetting([]string{"config", "set", "ui.archo.motion", "{}"}, "invalid"); err == nil {
		t.Fatal("GUI accepted invalid motion")
	}
	if got := api.Init().ArchoMotion; got != "live" {
		t.Fatalf("invalid motion overwrote saved value: %q", got)
	}
}
