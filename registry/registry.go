package registry

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type action string

const (
	ADD    action = "add"
	ALIVE  action = "alive"
	DELETE action = "delete"
)

const RegAddr string = "http://localhost:9999/_grpc_/registry"

type Registry struct {
	timeout time.Duration
	mu      sync.Mutex
	servers map[string]*serverItem
}

type serverItem struct {
	Addr  string
	start time.Time
}

const (
	defaultPath    = "/_grpc_/registry"
	defaultTimeout = 5 * time.Minute
)

func NewRegistry(timeout time.Duration) *Registry {
	return &Registry{
		timeout: timeout,
		servers: make(map[string]*serverItem),
	}
}

// 更新服务
func (r *Registry) updateServer(action action, addr string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch action {
	case ADD:
		r.servers[addr] = &serverItem{
			Addr:  addr,
			start: time.Now(),
		}
		log.Println(addr, "已注册")
	case ALIVE:
		r.servers[addr].start = time.Now()
	case DELETE:
		delete(r.servers, addr)
	}
}

func (r *Registry) aliveServers() []string {
	alive := make([]string, 0, len(r.servers))
	for addr, s := range r.servers {
		if s.start.Add(r.timeout).After(time.Now()) {
			alive = append(alive, addr)
		} else {
			r.updateServer(DELETE, addr)
		}
	}
	return alive
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case "GET":
		_, err := fmt.Fprint(w, r.aliveServers())
		if err != nil {
			return
		}
	case "POST":
		err := req.ParseForm()
		if err != nil {
			return
		}
		newServer := req.Form.Get("newServer")
		if newServer != "" {
			r.updateServer(ADD, newServer)
			return
		}

		deleteServer := req.Form.Get("deleteServer")
		if deleteServer != "" {
			r.updateServer(DELETE, deleteServer)
			return
		}

		alive := req.Form.Get("alive")
		if alive != "" {
			r.updateServer(ALIVE, alive)
			return
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *Registry) handleHTTP(registryPath string) {
	http.Handle(registryPath, r)
}

func (r *Registry) Run(addr string) {
	mux := &http.ServeMux{}
	mux.Handle(defaultPath, r)
	srv := http.Server{
		Addr:    addr,
		Handler: mux,
	}
	fmt.Println("启动!")
	err := srv.ListenAndServe()

	if err != nil {
		fmt.Println("注册中心启动失败:", err)
		return
	}
}

func HeartBeat(registryAddr, srvAddr string) {
	t := time.NewTicker(4 * time.Minute)
	var err error
	for err == nil {
		<-t.C
		err = sendHeartBeat(registryAddr, srvAddr)
	}
}

func sendHeartBeat(registryAddr, srvAddr string) error {
	log.Println("发送心跳包:", registryAddr)
	data := "alive=" + srvAddr
	_, err := http.Post(registryAddr, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		fmt.Println("心跳包发送失败:", err)
		return err
	}
	return nil
}

func RegisterServer(registryAddr, srvAddr string) error {
	data := "newServer=" + srvAddr
	_, err := http.Post(registryAddr, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		fmt.Println("注册服务失败:")
		return err
	}

	go HeartBeat(registryAddr, srvAddr)
	return nil
}

func OfflineServer(registryAddr, srvAddr string) error {
	data := "deleteServer=" + srvAddr
	_, err := http.Post(registryAddr, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		fmt.Println("注册服务失败:", err)
		return err
	}
	return nil
}
