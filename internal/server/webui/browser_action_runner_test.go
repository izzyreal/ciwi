package webui

import (
	"os/exec"
	"testing"
)

func TestBrowserActionRunnerBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; skipping executable browser action runner test")
	}
	command := exec.Command(node, "--test", "testdata/actions_test.js")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser action runner tests failed: %v\n%s", err, output)
	}
}
