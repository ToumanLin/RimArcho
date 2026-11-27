package view

import "testing"

// 16 §2.14 "Layout": a group of sections takes the strongest dot of its sections, amber over
// the accent, and the first meter.
func TestMergeGroupsAttention(t *testing.T) {
	d := &DiskMeter{Free: 1}
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{AttentionNone, AttentionNone}, AttentionNone},
		{[]string{AttentionAccent, AttentionNone}, AttentionAccent},
		{[]string{AttentionAccent, AttentionAmber}, AttentionAmber},
		{[]string{AttentionAmber, AttentionAccent}, AttentionAmber},
	} {
		var secs []SettingGroup
		for i, a := range c.in {
			secs = append(secs, SettingGroup{Key: string(rune('a' + i)), Attention: a})
		}
		secs[1].Disk = d
		g := mergeGroups("x", []string{"y"}, secs...)
		if g.Attention != c.want || g.Key != "a" || g.Disk != d || len(g.Sections) != len(c.in) {
			t.Errorf("%v: %+v", c.in, g)
		}
	}
}
