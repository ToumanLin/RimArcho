package view

import "testing"

func TestSelfUpdateSettingCommands(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		task string
	}{
		{[]string{"update"}, TaskSelfUpdate},
		{[]string{"update", "--check"}, TaskSelfCheck},
		{[]string{"update", "--from", `D:\Downloads\release (1).zip`}, TaskSelfUpdate},
		{[]string{"update", "--from"}, ""},
		{[]string{"update", "--from", ""}, ""},
		{[]string{"update", "--from", "release.zip", "extra"}, ""},
	} {
		if task := SettingTask(tc.argv); task != tc.task {
			t.Fatalf("%v: task %q", tc.argv, task)
		}
		if err := SettingCommand(tc.argv); (err == nil) != (tc.task != "") {
			t.Fatalf("%v: %v", tc.argv, err)
		}
	}
}
