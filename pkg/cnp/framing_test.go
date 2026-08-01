package cnp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"

	cnpv1 "github.com/izzyreal/ciwi/pkg/cnp/v1"
)

func TestFrameRoundTripWithShortWrites(t *testing.T) {
	w := &shortWriter{limit: 2}
	welcome := &cnpv1.Welcome{ServerName: "ciwi", ServerVersion: "v0.2.0"}
	if err := Write(w, welcome); err != nil {
		t.Fatal(err)
	}

	var decoded cnpv1.Welcome
	if err := NewReader(bytes.NewReader(w.buffer.Bytes())).Read(&decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ServerName != welcome.ServerName || decoded.ServerVersion != welcome.ServerVersion {
		t.Fatalf("decoded welcome = %s", decoded.String())
	}
}

func TestReaderRejectsInvalidFrameSizes(t *testing.T) {
	for _, tc := range []struct {
		name string
		size uint64
	}{
		{name: "zero", size: 0},
		{name: "oversized", size: MaxControlFrameSize + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var framed bytes.Buffer
			if err := binary.Write(&framed, binary.LittleEndian, []byte{}); err != nil {
				t.Fatal(err)
			}
			var prefix [binary.MaxVarintLen64]byte
			n := binary.PutUvarint(prefix[:], tc.size)
			framed.Write(prefix[:n])
			var message cnpv1.Empty
			err := NewReader(&framed).Read(&message)
			if err == nil || !strings.Contains(err.Error(), "invalid frame size") {
				t.Fatalf("Read() error = %v", err)
			}
		})
	}
}

func TestReaderReportsTruncatedFrame(t *testing.T) {
	var framed bytes.Buffer
	framed.WriteByte(5)
	framed.Write([]byte{1, 2})
	var message cnpv1.Empty
	err := NewReader(&framed).Read(&message)
	if err == nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Read() error = %v", err)
	}
}

type shortWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (w *shortWriter) Write(payload []byte) (int, error) {
	if len(payload) > w.limit {
		payload = payload[:w.limit]
	}
	return w.buffer.Write(payload)
}
