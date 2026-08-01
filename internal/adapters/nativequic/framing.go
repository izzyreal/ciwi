package nativequic

import (
	"io"

	"github.com/izzyreal/ciwi/pkg/cnp"
	"google.golang.org/protobuf/proto"
)

const MaxControlFrameSize = cnp.MaxControlFrameSize

type frameReader = cnp.Reader

func newFrameReader(reader io.Reader) *frameReader { return cnp.NewReader(reader) }

func writeFrame(writer io.Writer, message proto.Message) error { return cnp.Write(writer, message) }
