package server

import "testing"

func TestParseJobExecutionBuildMetadataFromOutput(t *testing.T) {
	out := `
noise
__CIWI_BUILD_SUMMARY__ target=linux-amd64 version=v1.2.3 output=dist/ciwi-linux-amd64
more noise
`
	meta := parseJobExecutionBuildMetadataFromOutput(out)
	if meta["build_target"] != "linux-amd64" {
		t.Fatalf("unexpected build_target: %q", meta["build_target"])
	}
	if meta["build_version"] != "v1.2.3" {
		t.Fatalf("unexpected build_version: %q", meta["build_version"])
	}
}
