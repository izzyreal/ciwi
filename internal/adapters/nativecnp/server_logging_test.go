package nativecnp

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

func TestSlowNativeRequestLogContainsMetadataWithoutPayloads(t *testing.T) {
	var output bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })
	request := &cnpv1.Request{
		Metadata: &cnpv1.RequestMetadata{RequestId: "request-1", TimeoutMs: 8000},
		Operation: &cnpv1.Request_GetProjectIcons{GetProjectIcons: &cnpv1.GetProjectIconsRequest{
			ProjectIds: []int64{7, 8},
		}},
	}
	logNativeUnaryRequest(context.Background(), request, &cnpv1.Response{RequestId: "request-1"}, 1500*time.Millisecond)
	logged := output.String()
	for _, expected := range []string{`"operation":"get_project_icons"`, `"request_id":"request-1"`, `"timeout_ms":8000`, `"requested_icon_count":2`, `"elapsed_ms":1500`} {
		if !strings.Contains(logged, expected) {
			t.Errorf("log missing %s: %s", expected, logged)
		}
	}
	for _, forbidden := range []string{"project_ids", "icon_bytes", "ssh", "credential"} {
		if strings.Contains(strings.ToLower(logged), forbidden) {
			t.Errorf("log contains payload field %q: %s", forbidden, logged)
		}
	}
}

func TestFastSuccessfulNativeRequestIsNotLogged(t *testing.T) {
	var output bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(original) })
	request := &cnpv1.Request{
		Metadata:  &cnpv1.RequestMetadata{RequestId: "request-2", TimeoutMs: 8000},
		Operation: &cnpv1.Request_GetServerInfo{GetServerInfo: &cnpv1.Empty{}},
	}
	logNativeUnaryRequest(context.Background(), request, &cnpv1.Response{RequestId: "request-2"}, 20*time.Millisecond)
	if output.Len() != 0 {
		t.Fatalf("fast request logged: %s", output.String())
	}
}
