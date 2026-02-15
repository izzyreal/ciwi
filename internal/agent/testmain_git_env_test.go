package agent

import (
	"os"
	"testing"

	"github.com/izzyreal/ciwi/internal/testutil"
)

func TestMain(m *testing.M) {
	restore := testutil.SetGitEnvHardening()
	code := m.Run()
	restore()
	os.Exit(code)
}
