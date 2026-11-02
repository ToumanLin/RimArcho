package cli

import (
	"strings"
	"testing"
)

// --open-plan names the list a command acts on without making it current (04 §1.3).
func TestOpenPlanFlag(t *testing.T) {
	for _, c := range []struct {
		argv       []string
		plan, rest string
	}{
		{[]string{"analyze", "--open-plan", "B"}, "B", "analyze"},
		{[]string{"--open-plan=Two words", "pack", "build", "p"}, "Two words", "pack build p"},
		{[]string{"textures", "optimize", "--plan"}, "", "textures optimize --plan"},
		{[]string{"x", "--", "--open-plan", "B"}, "", "x -- --open-plan B"},
	} {
		plan, rest, err := openPlanFlag(c.argv)
		if err != nil || plan != c.plan || strings.Join(rest, " ") != c.rest {
			t.Fatalf("%q: %q %q %v", c.argv, plan, rest, err)
		}
	}
	if _, _, err := openPlanFlag([]string{"analyze", "--open-plan"}); err == nil {
		t.Fatal("--open-plan without a name")
	}
}

func TestHereCompatibility(t *testing.T) {
	for _, tc := range []struct {
		argv      []string
		env, rest string
	}{
		{[]string{"--here"}, "", ""},
		{[]string{"--here", "tui"}, "", "tui"},
		{[]string{"--env", "main", "--here", "status"}, "main", "status"},
		{[]string{"list", "--here"}, "", "list --here"},
		{[]string{"--", "--here"}, "", "-- --here"},
	} {
		env, rest, err := globalFlags(tc.argv)
		if err != nil || env != tc.env || strings.Join(rest, " ") != tc.rest {
			t.Fatalf("%q: env=%q rest=%q err=%v", tc.argv, env, rest, err)
		}
	}
}
