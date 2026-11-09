package dotnet

import (
	"path/filepath"
	"reflect"
	"testing"
)

func accessBin(p ...string) string {
	return filepath.Join(append([]string{"..", "..", "testdata", "dotnet", "bin", "access"}, p...)...)
}

// A Mod compiled against a publicized game: the patch class's protected field read and internal
// call are refused unless the build skips verification; a subclass's read of the protected
// field is not (MCP eval h8: csc builds of Progression: Education threw FieldAccessException).
func TestInaccessibleRefs(t *testing.T) {
	game, err := Open(accessBin("Assembly-CSharp.dll"))
	if err != nil {
		t.Fatal(err)
	}
	open := func(name string) *Assembly {
		if name == "Assembly-CSharp" {
			return game
		}
		return nil
	}
	refs := func(v string) (*Assembly, []string) {
		a, err := Open(accessBin(v, "AccessMod.dll"))
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, r := range a.InaccessibleRefs(open) {
			out = append(out, r.Text+" "+r.Access)
		}
		return a, out
	}
	plain, got := refs("plain")
	want := []string{"Verse.StatWorker::Secret() internal", "Verse.StatWorker::stat protected"}
	if !reflect.DeepEqual(got, want) || plain.SkipsVerification() {
		t.Errorf("plain: %v skips %v, want %v", got, plain.SkipsVerification(), want)
	}
	unsafe, got := refs("unsafe")
	if !reflect.DeepEqual(got, want) || !unsafe.SkipsVerification() {
		t.Errorf("unsafe: %v skips %v", got, unsafe.SkipsVerification())
	}
	if _, got := refs("sub"); len(got) != 0 {
		t.Errorf("a subclass reading the protected field: %v", got)
	}
	// Without the game the references cannot be judged.
	if got := plain.InaccessibleRefs(func(string) *Assembly { return nil }); len(got) != 0 {
		t.Errorf("no game: %v", got)
	}
	r := Diff(unsafe, plain, open)
	if r.SkipsVerification || !r.OldSkipsVerification || len(r.Inaccessible) != 2 {
		t.Errorf("diff unsafe → plain: skips %v old %v inaccessible %v", r.SkipsVerification, r.OldSkipsVerification, r.Inaccessible)
	}
	if r := Diff(plain, unsafe, open); !r.SkipsVerification || len(r.Inaccessible) != 0 {
		t.Errorf("diff plain → unsafe: skips %v inaccessible %v", r.SkipsVerification, r.Inaccessible)
	}
}
