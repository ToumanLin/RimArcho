package gui

import (
	"os/exec"
	"testing"
)

func TestDataTables(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed for the data table behavior tests")
	}
	if out, err := exec.Command(node, "--experimental-vm-modules", "--test", "testdata/datatables.mjs").CombinedOutput(); err != nil {
		t.Fatalf("data tables: %v\n%s", err, out)
	}
}
