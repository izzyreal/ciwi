package server

import "testing"

func TestSourceRefMatchesCachedNormalizesBranchRefsOnly(t *testing.T) {
	tests := []struct {
		name   string
		asked  string
		cached string
		want   bool
	}{
		{"both empty", "", "", true},
		{"one empty", "main", "", false},
		{"trimmed exact", " main ", "main", true},
		{"requested full branch", "refs/heads/main", "main", true},
		{"cached full branch", "main", "refs/heads/main", true},
		{"different branch", "main", "release", false},
		{"tags are not branch-normalized", "refs/tags/v1.0.0", "v1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceRefMatchesCached(tt.asked, tt.cached); got != tt.want {
				t.Fatalf("sourceRefMatchesCached(%q, %q)=%v, want %v", tt.asked, tt.cached, got, tt.want)
			}
		})
	}
}
