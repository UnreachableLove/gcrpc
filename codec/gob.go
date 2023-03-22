package codec

import (
	"encoding/gob"
	"io"
)

type GobCodec struct {
}

var _ Codec = (*GobCodec)(nil)

func (g GobCodec) Encode(e any, buf io.Writer) error {
	enc := gob.NewEncoder(buf)
	err := enc.Encode(e)
	if err != nil {
		return err
	}
	return nil
}

func (g GobCodec) Decode(buf io.Reader, e any) error {
	dec := gob.NewDecoder(buf)
	err := dec.Decode(e)
	if err != nil {
		return err
	}
	return nil
}
