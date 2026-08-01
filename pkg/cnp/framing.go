package cnp

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

const MaxControlFrameSize = 8 << 20

type Reader struct {
	reader *bufio.Reader
}

func NewReader(reader io.Reader) *Reader {
	return &Reader{reader: bufio.NewReader(reader)}
}

func (r *Reader) Read(message proto.Message) error {
	size, err := binary.ReadUvarint(r.reader)
	if err != nil {
		return fmt.Errorf("read frame size: %w", err)
	}
	if size == 0 || size > MaxControlFrameSize {
		return fmt.Errorf("invalid frame size %d", size)
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r.reader, payload); err != nil {
		return fmt.Errorf("read frame payload: %w", err)
	}
	if err := proto.Unmarshal(payload, message); err != nil {
		return fmt.Errorf("decode protobuf frame: %w", err)
	}
	return nil
}

func Write(writer io.Writer, message proto.Message) error {
	payload, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode protobuf frame: %w", err)
	}
	if len(payload) == 0 || len(payload) > MaxControlFrameSize {
		return fmt.Errorf("invalid encoded frame size %d", len(payload))
	}
	var prefix [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(prefix[:], uint64(len(payload)))
	if err := writeAll(writer, prefix[:n]); err != nil {
		return fmt.Errorf("write frame size: %w", err)
	}
	if err := writeAll(writer, payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

func writeAll(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := writer.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}
