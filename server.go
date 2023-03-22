package grpc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"grpc/codec"
	. "grpc/registry"
	"io"
	"log"
	"net"
	"strings"
)

type singleConn struct {
	conn          net.Conn
	codec         codec.Codec
	responseQueue chan responseData
	closed        chan bool
}

type Response struct {
	Seq    uint64
	Error  int
	Result []any
}

type responseData struct {
	size     uint64
	response []byte
}

type Server struct {
	serviceIp   string
	servicePort int
	addr        string
	ln          net.Listener
}

func NewServer(ip string, port int) *Server {
	return &Server{
		serviceIp:   ip,
		servicePort: port,
		addr:        fmt.Sprintf("%s:%d", ip, port),
	}
}

// Start 开启监听
func (srv *Server) Start() error {

	ln, err := net.Listen("tcp", srv.addr)
	if err != nil {
		log.Println("rpc client: net.listen error:", err)
		return err
	}
	srv.ln = ln

	err = RegisterServer(RegAddr, srv.addr)
	if err != nil {
		return err
	}

	return srv.serve()
}

// serve 循环接收连接
func (srv *Server) serve() error {
	fmt.Println("开始监听...")
	for {
		conn, err := srv.ln.Accept()
		if err != nil {
			log.Println("rpc client: net.listen error:", err)
			return err
		}
		fmt.Println()
		fmt.Println("请求已接收:", conn.RemoteAddr())
		go serveConn(conn)
	}
}

// serveConn 处理单个连接
// 读取协议头进行校验
// 循环读取单个调用
func serveConn(conn net.Conn) {
	//读取协议头
	header, err := ReadHeader(conn)
	if err != nil {
		log.Fatalln("server read header error:", err)
	}
	//验证数据合法性
	if !header.CheckMagicNumber() {
		log.Fatalln("协议校验未通过")
	}

	fmt.Println("协议校验通过")

	sc := &singleConn{
		conn:          conn,
		codec:         codec.NewCodecMap[header.SerializeType],
		responseQueue: make(chan responseData, 10),
		closed:        make(chan bool, 1),
	}

	//使用两个协程分别读取请求和发送结果
	go sc.readRequest()
	go sc.safeSend()

}

func (sc *singleConn) readRequest() {
	//读取onecall大小
	var size uint64

	for {
		if err := binary.Read(sc.conn, binary.BigEndian, &size); err != nil {
			if err == io.EOF {
				err = sc.close()
				if err != nil {
					log.Println("关闭 singleConn 失败：", err)
				}
				break
			} else if strings.Contains(err.Error(), "use of closed network connection") {
				break
			} else {
				log.Println("onecall 大小读取失败，读取下一个", err)
				continue
			}
		}
		//从流中读取一个 onecall
		buf := make([]byte, size)
		_, err := io.ReadFull(sc.conn, buf)
		if err != nil {
			if err == io.EOF {
				err = sc.close()
				if err != nil {
					log.Println("关闭 singleConn 失败：", err)
				}
				break
			} else if strings.Contains(err.Error(), "use of closed network connection") {
				break
			} else {
				log.Println("onecall 读取失败，读取下一个", err)
				continue
			}
		}
		go sc.handleRequest(buf)
	}

	log.Println("读取协程退出")
}

func (sc *singleConn) handleRequest(request []byte) {

	//将读取的[]byte反序列化为onecall
	req := &Request{}
	if err := sc.codec.Decode(bytes.NewReader(request), req); err != nil {
		log.Println("反序列化请求错误:", err)
	}

	re := &Response{Seq: req.Seq, Error: 0}
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
	for i := 0; i < len(out); i++ {
		re.Result = append(re.Result, out[i].Interface())
	}
	sc.handleResponse(re)
}

// 将返回值序列化为[]byte
func (sc *singleConn) handleResponse(re *Response) {
	buf := new(bytes.Buffer)
	err := sc.codec.Encode(re, buf)
	if err != nil {
		log.Println("序列化返回值错误:", err)
	}
	//调用发送函数
	sc.sendResponse(buf.Bytes())
}

func (sc *singleConn) sendResponse(buf []byte) {

	size := uint64(len(buf))
	respData := responseData{
		size:     size,
		response: buf,
	}
	sc.responseQueue <- respData

}

func (sc *singleConn) safeSend() {
	for {
		select {
		case respData := <-sc.responseQueue:
			if err := binary.Write(sc.conn, binary.BigEndian, respData.size); err != nil {
				log.Println("发送返回值大小出现错误:", err)
				continue
			}
			_, err := sc.conn.Write(respData.response)
			if err != nil {
				log.Println("发送返回值出现错误:", err)
				continue
			}
			fmt.Println("已发送返回值")
		case <-sc.closed:
			log.Println("发送协程退出")
			return
		}
	}
}

func (sc *singleConn) close() error {
	sc.closed <- true
	err := sc.conn.Close()
	if err != nil {
		return err
	}
	log.Println("socket 已关闭")
	return nil
}
