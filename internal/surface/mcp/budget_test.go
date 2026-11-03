package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ToumanLin/RimArcho/internal/cmds"
	"github.com/ToumanLin/RimArcho/internal/surface/mcp"
	"github.com/ToumanLin/RimArcho/internal/testkit"
)

// 05 §5.3: what a client reads when it connects stays short; the detail is in guide topics.
// The character limits apply to the default (core) tools; the tools/list size to all of them.
// Raise a limit only on purpose, after making the new text as short as the rest.
const (
	maxInstructions = 2000     // characters
	maxDescription  = 320      // characters, any one tool
	maxBrief        = 12000    // characters: Instructions plus every tool description
	maxToolsList    = 48 << 10 // bytes: the tools/list result as JSON, schemas included
)

func TestInitialContextBudget(t *testing.T) {
	e := testkit.New(t)
	cs := connectGroup(t, e, mcp.Core)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	instr := utf8.RuneCountInString(cs.InitializeResult().Instructions)
	if instr > maxInstructions {
		t.Errorf("Instructions: %d characters, limit %d", instr, maxInstructions)
	}
	brief := instr
	for _, tl := range res.Tools {
		n := utf8.RuneCountInString(tl.Description)
		brief += n
		if n > maxDescription {
			t.Errorf("%s: description %d characters, limit %d (move the detail to its guide topic)", tl.Name, n, maxDescription)
		}
	}
	if brief > maxBrief {
		t.Errorf("Instructions and descriptions: %d characters, limit %d", brief, maxBrief)
	}
	core, _ := json.Marshal(res)
	all, err := connect(t, e).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	list, _ := json.Marshal(all)
	if len(list) > maxToolsList {
		t.Errorf("tools/list (--tools all): %d bytes, limit %d", len(list), maxToolsList)
	}
	t.Logf("core: instructions %d + descriptions %d = %d characters; tools/list %d bytes, %d tools; all: tools/list %d bytes, %d tools",
		instr, brief-instr, brief, len(core), len(res.Tools), len(list), len(all.Tools))
}

// Every guide topic the topic list names is served by the guide tool, and the list names
// every tool but guide itself.
func TestGuideTopics(t *testing.T) {
	e := testkit.New(t)
	cs := connect(t, e)
	out, isErr := call(t, cs, "guide", nil)
	if isErr {
		t.Fatalf("guide:\n%s", out)
	}
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range res.Tools {
		if tl.Name != "guide" && !strings.Contains(out, " "+tl.Name) {
			t.Errorf("topic list misses tool %s", tl.Name)
		}
	}
	for _, topic := range cmds.GuideTopics() {
		text, isErr := call(t, cs, "guide", map[string]any{"topic": topic})
		want, _ := cmds.Guide(topic)
		if isErr || !strings.HasPrefix(text, "guide: "+topic+"\n") || !strings.Contains(text, strings.TrimSpace(want)) {
			t.Errorf("guide topic=%s: isError=%v\n%s", topic, isErr, text)
		}
		if !strings.Contains(out, topic) {
			t.Errorf("topic list misses %s", topic)
		}
	}
	if out, isErr := call(t, cs, "guide", map[string]any{"topic": "nope"}); !isErr || !strings.Contains(out, "unknown guide topic") {
		t.Errorf("unknown topic:\n%s", out)
	}
}
