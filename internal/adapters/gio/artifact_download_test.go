//go:build darwin || ios || linux || windows

package gio

import (
	"bytes"
	"context"
	"io"
	"testing"

	"gioui.org/x/explorer"
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

type artifactPickerStub struct {
	name   string
	writer io.WriteCloser
	err    error
}

func (p *artifactPickerStub) CreateFile(name string) (io.WriteCloser, error) {
	p.name = name
	return p.writer, p.err
}

func TestDownloadArtifactUsesNativePickerAndStreamsContent(t *testing.T) {
	client := &artifactChunkClientStub{}
	writer := &artifactWriter{}
	picker := &artifactPickerStub{writer: writer}
	name, err := downloadArtifactWithPicker(t.Context(), client, picker, map[string]string{
		"jobExecutionId": "job-1", "kind": "file", "path": "dist/app.zip",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "app.zip" || picker.name != "app.zip" {
		t.Fatalf("download name = %q, picker suggestion = %q", name, picker.name)
	}
	if writer.String() != "first-second" || !writer.closed || len(client.requests) != 2 {
		t.Fatalf("download = %q, closed = %t, requests = %d", writer.String(), writer.closed, len(client.requests))
	}
}

func TestDownloadJobLogUsesSharedNativeDownloadTransport(t *testing.T) {
	client := &artifactChunkClientStub{}
	writer := &artifactWriter{}
	picker := &artifactPickerStub{writer: writer}
	_, err := downloadArtifactWithPicker(t.Context(), client, picker, map[string]string{
		"jobExecutionId": "job-1", "kind": "log-clean",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) == 0 || client.requests[0].GetKind() != "log-clean" {
		t.Fatalf("log download requests = %+v", client.requests)
	}
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

func TestDownloadArtifactCancellationAbortsServerSession(t *testing.T) {
	client := &artifactChunkClientStub{}
	picker := &artifactPickerStub{err: explorer.ErrUserDecline}
	_, err := downloadArtifactWithPicker(t.Context(), client, picker, map[string]string{
		"jobExecutionId": "job-1", "kind": "all",
	})
	if err != errArtifactDownloadCancelled {
		t.Fatalf("download error = %v", err)
	}
	if len(client.requests) != 2 || !client.requests[1].GetCancel() || client.requests[1].GetToken() != "token" {
		t.Fatalf("cancellation requests = %+v", client.requests)
	}
}
