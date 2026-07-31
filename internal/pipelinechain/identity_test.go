package pipelinechain

import "testing"

func TestIDIsStableOrderedAndBoundarySafe(t *testing.T) {
	base := ID([]string{"build", "release"})
	if base != ID([]string{" build ", "release"}) {
		t.Fatalf("expected surrounding whitespace to be normalized")
	}
	if base == ID([]string{"release", "build"}) {
		t.Fatalf("expected pipeline order to affect identity")
	}
	if ID([]string{"a", "bc"}) == ID([]string{"ab", "c"}) {
		t.Fatalf("expected pipeline boundaries to affect identity")
	}
	if len(base) != len("chain-")+sha256HexLength {
		t.Fatalf("unexpected id length: %q", base)
	}
}

func TestDisplayNameUsesConfiguredNameOrSequence(t *testing.T) {
	if got := DisplayName(" Full release ", []string{"build", "release"}); got != "Full release" {
		t.Fatalf("unexpected configured name: %q", got)
	}
	if got := DisplayName("", []string{"build", "release"}); got != "build → release" {
		t.Fatalf("unexpected derived name: %q", got)
	}
}

const sha256HexLength = 64
