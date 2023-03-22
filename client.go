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
	"sync"
	"sync/atomic"
	"time"
)

// type TimeOut bool
//
// const timeOut TimeOut = true
const ConnectTimeout = 2 * time.Second

type TimeOutErr struct {
	error string
}

func (t TimeOutErr) Error() string {
	return "超时！"
}

type MethodNotFoundErr struct {
	error string
}

func (m MethodNotFoundErr) Error() string {
	return "未找到该方法！"
}

type Client struct {
	conn         net.Conn
	h            Header
	codec        codec.Codec
	totalCalls   uint64
	requestQueue chan requestData
	closed       chan bool
	results      map[uint64]*OneCall
	balancer     *Balancer
}

type Request struct {
	Seq           uint64
	ServiceMethod string
	Args          []any
}

type OneCall struct {
	sync.Mutex
	closed      bool
	ResultQueue chan any
	Req         Request
}

type requestData struct {
	size    uint64
	request []byte
}

func NewClient(serializeType codec.SerializeType) *Client {
	return &Client{
		h: Header{
			MagicNumber:   magicNumber,
			SerializeType: serializeType,
		},
		codec:        codec.NewCodecMap[serializeType],
		totalCalls:   0,
		requestQueue: make(chan requestData, 10),
		closed:       make(chan bool, 1),
		results:      make(map[uint64]*OneCall),
		balancer:     NewBalancer(RegAddr),
	}
}

// Connect 和服务端建立tcp连接，发送协议头部，协商序列化方法
func (c *Client) Connect(addr string) error {
	//建立连接
	//conn, err := net.Dial("tcp", addr)
	addr, err := c.balancer.Get()
	conn, err := net.DialTimeout("tcp", addr, ConnectTimeout)
	if err != nil {
		return err
	}
	c.conn = conn
	fmt.Println("本机:", conn.LocalAddr(), " 已连接:", conn.RemoteAddr())
	//发送协议头
	if err := c.h.SendHeader(c.conn); err != nil {
		return err
	}
	go c.safeSend()
	go c.readResult()
	return nil
}

func (c *Client) Call(Method string, Args ...any) *OneCall {
	//一个call代表一个实际调用

	//序列化请求
	atomic.AddUint64(&(c.totalCalls), 1)
	o := OneCall{
		ResultQueue: make(chan any, 1),
		Req: Request{
			Seq:           atomic.LoadUint64(&(c.totalCalls)),
			ServiceMethod: Method,
			Args:          Args,
		},
	}

	buf := new(bytes.Buffer)
	if err := c.codec.Encode(o.Req, buf); err != nil {
		log.Fatalln("序列化请求错误:", err)
	}
	c.sendRequest(buf.Bytes())

	c.results[o.Req.Seq] = &o
	return &o
}

func (c *Client) CallWithTimeOut(outTime time.Duration, Method string, Args ...any) *OneCall {
	atomic.AddUint64(&(c.totalCalls), 1)
	o := OneCall{
		ResultQueue: make(chan any, 1),
		Req: Request{
			Seq:           atomic.LoadUint64(&(c.totalCalls)),
			ServiceMethod: Method,
			Args:          Args,
		},
	}

	buf := new(bytes.Buffer)
	if err := c.codec.Encode(o.Req, buf); err != nil {
		log.Fatalln("序列化请求错误:", err)
	}
	c.sendRequest(buf.Bytes())

	time.AfterFunc(outTime, func() {
		o.Lock()
		defer o.Unlock()
		if !o.closed {
			o.closed = true
			o.ResultQueue <- TimeOutErr{}
		}
	})

	c.results[o.Req.Seq] = &o

	return &o
}

// 将序列化后的请求依次放入请求队列中
func (c *Client) sendRequest(buf []byte) {
	size := uint64(len(buf))
	reqData := requestData{
		size:    size,
		request: buf,
	}
	c.requestQueue <- reqData

}

// 从请求队列中依次读取数据，发送给服务端
func (c *Client) safeSend() {
	for {
		select {
		case reqData := <-c.requestQueue:
			err := binary.Write(c.conn, binary.BigEndian, reqData.size)
			if err != nil {
				log.Println("发送请求大小错误:", err)
				continue
			}
			_, err = c.conn.Write(reqData.request)
			if err != nil {
				log.Println("发送请求错误:", err)
				continue
			}
		case <-c.closed:
			log.Println("发送协程退出")
			return
		}
	}
}

// 循环从conn中读取返回数据，每个返回数据使用一个协程进行处理
func (c *Client) readResult() {
	var size uint64
	for {
		err := binary.Read(c.conn, binary.BigEndian, &size)
		if err != nil {
			if err == io.EOF {
				//EOF 说明对端关闭了 socket，需要通知退出
				err = c.Close()
				if err != nil {
					log.Println("关闭 Client 失败：", err)
				}
				break
			} else if strings.Contains(err.Error(), "use of closed network connection") {
				//use of closed network connection 说明己方关闭了 socket
				//不需要通知，因为己方已经通知了
				break
			} else {
				log.Println("读取结果大小错误:", err)
				continue
			}
		}
		buf := make([]byte, size)
		_, err = io.ReadFull(c.conn, buf)
		if err != nil {
			if err == io.EOF {
				err = c.Close()
				if err != nil {
					log.Println("关闭 Client 失败：", err)
				}
				break
			} else if strings.Contains(err.Error(), "use of closed network connection") {
				break
			} else {
				log.Println("读取结果大小错误:", err)
				continue
			}
		}
		go c.handleResult(buf)
	}
	log.Println("读取协程退出")
}

// 将返回数据反序列化后，根据序号写入到对应的结果通道中
func (c *Client) handleResult(buf []byte) {
	result := new(Response)
	if err := c.codec.Decode(bytes.NewReader(buf), result); err != nil {
		fmt.Println("反序列化返回值错误:", err)
		return
	}
	o := c.results[result.Seq]
	o.Lock()
	defer o.Unlock()
	if !o.closed {
		o.closed = true
		if result.Error == 1 {
			c.results[result.Seq].ResultQueue <- MethodNotFoundErr{}
		}
		c.results[result.Seq].ResultQueue <- result.Result
	}

}

func (o *OneCall) Result() (any, error) {
	result := <-o.ResultQueue
	if err, ok := result.(error); !ok {
		res := result.([]any)
		n := len(res)
		e := res[n-1]
		if e == nil {
			return res[:n-1], nil
		} else {
			return nil, e.(error)
		}
	} else {
		if e, ok := err.(TimeOutErr); ok {
			return nil, e
		} else {
			return nil, err.(MethodNotFoundErr)
		}
	}
}

func (c *Client) Close() error {
	c.closed <- true
	err := c.conn.Close()
	if err != nil {
		return err
	}
	log.Println("socket 已关闭")
	return nil
}
