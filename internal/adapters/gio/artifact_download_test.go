//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"context"
	"testing"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
	"google.golang.org/protobuf/proto"
)

type artifactChunkClientStub struct {
	requests []*cnpv1.ArtifactDownloadRequest
}

func (s *artifactChunkClientStub) DownloadArtifactChunk(_ context.Context, request *cnpv1.ArtifactDownloadRequest) (*cnpv1.ArtifactDownloadChunk, error) {
	s.requests = append(s.requests, proto.Clone(request).(*cnpv1.ArtifactDownloadRequest))
	if request.GetCancel() {
		return &cnpv1.ArtifactDownloadChunk{Token: request.GetToken(), Complete: true}, nil
	}
	if request.GetToken() == "" {
		return &cnpv1.ArtifactDownloadChunk{Token: "token", FileName: "../app.zip", Data: []byte("first-"), NextOffset: 6, TotalSize: 12}, nil
	}
	if request.GetToken() != "token" || request.GetOffset() != 6 {
		return nil, context.Canceled
	}
	return &cnpv1.ArtifactDownloadChunk{Token: "token", FileName: "../app.zip", Data: []byte("second"), NextOffset: 12, TotalSize: 12, Complete: true}, nil
}

type artifactWriter struct {
	bytes.Buffer
	closed bool
}

func (w *artifactWriter) Close() error {
	w.closed = true
	return nil
}

func TestDownloadJobLogTextCollectsEveryTransportChunk(t *testing.T) {
	client := &artifactChunkClientStub{}
	text, err := downloadJobLogText(t.Context(), client, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if text != "first-second" || len(client.requests) != 2 || client.requests[0].GetKind() != "log-clean" {
		t.Fatalf("copied text = %q, requests = %+v", text, client.requests)
	}
}
