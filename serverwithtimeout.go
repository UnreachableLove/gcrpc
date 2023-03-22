package grpc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
)

func (sc *singleConn) readRequestWithTimeOut() {
	//读取onecall大小
	var size uint64

	for {
		outTime := time.Now().Add(time.Second)

		if err := binary.Read(sc.conn, binary.BigEndian, &size); err != nil {
			break
		}

		//从流中读取一个 onecall
		buf := make([]byte, size)
		_, err := io.ReadFull(sc.conn, buf)
		if err != nil {
			break
		}
		if outTime.After(time.Now()) {
			go sc.handleRequestWithTimeOut(buf, outTime)
		}

	}

	fmt.Println("读取大小出错，发送结束信号")
	sc.closed <- true

}

func (sc *singleConn) handleRequestWithTimeOut(request []byte, outTime time.Time) {

	//将读取的[]byte反序列化为onecall
	req := &Request{}
	if err := sc.codec.Decode(bytes.NewReader(request), req); err != nil {
		log.Println("反序列化请求错误:", err)
	}

	re := &Response{Seq: req.Seq}
	//调用请求的方法
	str := strings.Split(req.ServiceMethod, ".")
	svc, ok := RPCMethods[str[0]]
	if !ok {
		re.Error = 1
		sc.handleResponse(re)
		return
	}
	method, ok := svc.methods[str[1]]
	if !ok {
		re.Error = 1
		sc.handleResponse(re)
		return
	}
	out := svc.call(method, req.Args)
	//将返回值序列化为[]byte
	for i := 0; i < len(out); i++ {
		re.Result = append(re.Result, out[i].Interface())
	}
	buf := new(bytes.Buffer)
	err := sc.codec.Encode(re, buf)
	if err != nil {
		log.Println("序列化返回值错误:", err)
	}
	//调用发送函数
	if outTime.After(time.Now()) {
		sc.sendResponse(buf.Bytes())
	}
}
