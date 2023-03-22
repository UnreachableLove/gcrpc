package grpc

import (
	"encoding/json"
	"fmt"
	"grpc/codec"
	"io"
)

const magicNumber = 0x000913
const headerLength = 42

type Header struct {
	MagicNumber   int32
	SerializeType codec.SerializeType
}

func (h *Header) CheckMagicNumber() bool {
	return h.MagicNumber == magicNumber
}

func (h *Header) SendHeader(w io.Writer) error {

	hs, err := json.Marshal(h)
	if err != nil {
		return err
	}

	n, err := w.Write(hs)
	if err != nil {
		return err
	}
	fmt.Println("已发送协议头 字节数:", n)
	return nil
}

func ReadHeader(r io.Reader) (h *Header, err error) {
	h = new(Header)
	buf := make([]byte, headerLength)

	//读取固定长度协议头解码
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(buf, h)
	if err != nil {
		return nil, err
	}

	return h, nil
}
