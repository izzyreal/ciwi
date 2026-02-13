package server

import "testing"

func TestResolveEffectiveAgentUpdateTarget(t *testing.T) {
	tests := []struct {
		name           string
		configured     string
		serverVersion  string
		expectedTarget string
	}{
		{
			name:           "keeps update disabled when target empty",
			configured:     "",
			serverVersion:  "v1.2.0",
			expectedTarget: "",
		},
		{
			name:           "prefers newer server version over stale configured target",
			configured:     "v1.1.0",
			serverVersion:  "v1.2.0",
			expectedTarget: "v1.2.0",
		},
		{
			name:           "keeps configured target when it is newer",
			configured:     "v1.3.0",
			serverVersion:  "v1.2.0",
			expectedTarget: "v1.3.0",
		},
		{
			name:           "keeps configured target for dev server version",
			configured:     "v1.3.0",
			serverVersion:  "dev",
			expectedTarget: "v1.3.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveEffectiveAgentUpdateTarget(tt.configured, tt.serverVersion)
			if got != tt.expectedTarget {
				t.Fatalf("resolveEffectiveAgentUpdateTarget(%q, %q)=%q want=%q", tt.configured, tt.serverVersion, got, tt.expectedTarget)
			}
		})
	}
}

func TestResolveManualAgentUpdateTarget(t *testing.T) {
	tests := []struct {
		name           string
		serverVersion  string
		configured     string
		expectedTarget string
	}{
		{
			name:           "defaults to server version",
			serverVersion:  "v1.2.0",
			configured:     "",
			expectedTarget: "v1.2.0",
		},
		{
			name:           "uses configured target when newer than server version",
			serverVersion:  "v1.2.0",
			configured:     "v1.3.0",
			expectedTarget: "v1.3.0",
		},
		{
			name:           "ignores stale configured target",
			serverVersion:  "v1.2.0",
			configured:     "v1.1.0",
			expectedTarget: "v1.2.0",
		},
		{
			name:           "passes through dev version",
			serverVersion:  "dev",
			configured:     "v1.3.0",
			expectedTarget: "dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveManualAgentUpdateTarget(tt.serverVersion, tt.configured)
			if got != tt.expectedTarget {
				t.Fatalf("resolveManualAgentUpdateTarget(%q, %q)=%q want=%q", tt.serverVersion, tt.configured, got, tt.expectedTarget)
			}
		})
	}
}
