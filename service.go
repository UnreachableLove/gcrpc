package grpc

import (
	"reflect"
	"strings"
)

var RPCMethods map[string]*service

func init() {
	RPCMethods = make(map[string]*service)
	newService(MathObject{})
}

type service struct {
	name    string
	typ     reflect.Type
	rcv     reflect.Value
	methods map[string]*reflect.Method
}

func newService(object any) {
	s := new(service)
	s.typ = reflect.TypeOf(object)
	s.rcv = reflect.ValueOf(object)
	s.name = reflect.Indirect(s.rcv).Type().Name()

	s.registerMethods()
}

// 注册方法到方法表,将服务注册到服务表
func (s *service) registerMethods() {
	s.methods = make(map[string]*reflect.Method)
	for i := 0; i < s.typ.NumMethod(); i++ {
		m := s.typ.Method(i)
		s.methods[m.Name] = &m
	}

	//在全局表中注册service
	serviceName := s.name[:len(s.name)-6]
	RPCMethods[serviceName] = s
}

func (s *service) call(method *reflect.Method, args []any) []reflect.Value {
	n := len(args)
	in := make([]reflect.Value, n+1)
	for i := 0; i < n; i++ {
		in[i+1] = reflect.ValueOf(args[i])
	}
	in[0] = s.rcv

	return method.Func.Call(in)
}

type MathObject struct{}

func (MathObject) Add(a, b int) (int, error) {
	return a + b, nil
}
func (MathObject) Sub(a, b int) (int, error) {
	return a - b, nil
}

type StringObject struct {
}

func (MathObject) ToUpper(s string) (string, error) {
	return strings.ToUpper(s), nil
}
func (MathObject) ToLower(s string) (string, error) {
	return strings.ToLower(s), nil
}
