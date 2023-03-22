package codec

import "io"

type Codec interface {
	Encode(e any, buf io.Writer) error
	Decode(buf io.Reader, e any) error
}

type SerializeType string

const (
	GobType  SerializeType = "Gob"
	JSONType SerializeType = "JSON"
)

type NewCodecFunc func(io.ReadWriteCloser) Codec

var NewCodecMap map[SerializeType]Codec

func init() {
	NewCodecMap = make(map[SerializeType]Codec)
	NewCodecMap[GobType] = GobCodec{}
}
