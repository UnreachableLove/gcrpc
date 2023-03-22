package grpc

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Balance interface {
	refresh()
	update() error
	Get() (string, error)
	GetAll() ([]string, error)
}

type Balancer struct {
	addr    string
	mu      sync.Mutex
	timeout time.Duration
	r       *rand.Rand
	index   int
	closed  chan bool
	servers []string
}

var _ Balance = (*Balancer)(nil)

// 定时刷新
func (b *Balancer) refresh() {
	t := time.NewTicker(b.timeout)
	var err error
	for err == nil {
		select {
		case <-t.C:
			err = b.update()
		case <-b.closed:
			return
		}

	}
}

func (b *Balancer) update() error {

	//get 请求获取数据
	resp, err := http.Get(b.addr)
	if err != nil {
		fmt.Println("获取服务列表失败")
		return err
	}
	//二进制数据转换为列表
	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
		return err
	}
	str := string(buf)
	fmt.Println(str)
	str = str[1 : len(str)-1]
	if len(str) == 0 {
		log.Println("获取服务列表为空")
	}
	alive := strings.Split(str, " ")
	b.mu.Lock()
	b.servers = alive
	b.mu.Unlock()
	return nil
}

func (b *Balancer) Get() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(b.servers)
	if n == 0 {
		return "", errors.New("可用服务列表为空")
	}
	srv := b.servers[b.index%n]
	b.index = (b.index + 1) % n
	return srv, nil
}

func (b *Balancer) GetAll() ([]string, error) {
	return b.servers, nil
}

func NewBalancer(addr string) *Balancer {
	b := &Balancer{
		addr:    addr,
		timeout: 10 * time.Second,
		//如果种子相同，那么生成的随机序列也相同。所以要赋予不同的种子
		r:       rand.New(rand.NewSource(time.Now().UnixNano())),
		closed:  make(chan bool, 1),
		servers: make([]string, 0),
	}
	//避免每次从0开始，初始化时随机设置一个值
	b.index = b.r.Intn(math.MaxInt - 1)
	err := b.update()
	if err != nil {
		fmt.Println("创建均衡器失败")
		return nil
	}
	go b.refresh()
	return b
}

func (b *Balancer) Close() {
	b.closed <- true
}
