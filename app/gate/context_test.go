package gate

import (
	"errors"
	"testing"

	"github.com/2comjie/wali/core/endpoint"
)

func TestContextReply(t *testing.T) {
	tell := &Context{}
	if !errors.Is(tell.Reply([]byte("rsp")), ErrReplyNotAllowed) {
		t.Fatal("Tell请求可以调用Reply")
	}

	call := &Context{needReply: true}
	body := []byte("rsp")
	if err := call.Reply(body); err != nil {
		t.Fatal(err)
	}
	if !call.replied || string(call.responseBody) != "rsp" {
		t.Fatalf("响应没有保存: replied=%v body=%q", call.replied, call.responseBody)
	}
	if !errors.Is(call.Reply(body), ErrAlreadyReplied) {
		t.Fatal("同一请求可以重复调用Reply")
	}
}

func TestProxyInstanceReturnsCopy(t *testing.T) {
	proxy := &Proxy{
		app: &Gate{
			instance: endpoint.ServiceInstance{
				ID:       "gate-1",
				MetaData: map[string]string{"zone": "1"},
			},
		},
	}

	instance := proxy.Instance()
	instance.MetaData["zone"] = "2"
	if proxy.app.instance.MetaData["zone"] != "1" {
		t.Fatal("业务层可以修改Gate内部的实例元数据")
	}
}
