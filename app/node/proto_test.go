package node

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestReg(t *testing.T) {
	router := NewRouter()
	calls := 0
	router.Reg(1, func(ctx *Context, req *wrapperspb.StringValue, rsp *wrapperspb.StringValue) error {
		calls++
		if calls == 1 {
			rsp.Value = "hello " + req.Value
		}
		return nil
	})
	firstBody, err := proto.Marshal(wrapperspb.String("taoxi"))
	if err != nil {
		t.Fatal(err)
	}
	firstCtx := &Context{Request: &Request{Route: 1, Body: firstBody, NeedReply: true}}
	if err = router.Dispatch(firstCtx); err != nil {
		t.Fatal(err)
	}
	firstRsp := &wrapperspb.StringValue{}
	if err = proto.Unmarshal(firstCtx.ResponseBody(), firstRsp); err != nil {
		t.Fatal(err)
	}
	if firstRsp.Value != "hello taoxi" {
		t.Fatalf("response=%q", firstRsp.Value)
	}

	secondCtx := &Context{Request: &Request{Route: 1, NeedReply: true}}
	if err = router.Dispatch(secondCtx); err != nil {
		t.Fatal(err)
	}
	secondRsp := &wrapperspb.StringValue{}
	if err = proto.Unmarshal(secondCtx.ResponseBody(), secondRsp); err != nil {
		t.Fatal(err)
	}
	if secondRsp.Value != "" {
		t.Fatalf("response was reused: %q", secondRsp.Value)
	}
}

func TestRegTellDoesNotReply(t *testing.T) {
	router := NewRouter()
	router.Reg(1, func(ctx *Context, req *wrapperspb.StringValue, rsp *wrapperspb.StringValue) error {
		rsp.Value = req.Value
		return nil
	})
	body, err := proto.Marshal(wrapperspb.String("taoxi"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := &Context{Request: &Request{Route: 1, Body: body}}
	if err = router.Dispatch(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.ResponseBody() != nil {
		t.Fatal("Tell request must not have a response")
	}
}

func TestRegReturnsDecodeAndHandlerErrors(t *testing.T) {
	handlerErr := errors.New("handler failed")
	router := NewRouter()
	called := false
	router.Reg(1, func(ctx *Context, req *wrapperspb.StringValue, rsp *wrapperspb.StringValue) error {
		called = true
		return handlerErr
	})
	decodeErr := router.Dispatch(&Context{Request: &Request{Route: 1, Body: []byte{0xff}, NeedReply: true}})
	if decodeErr == nil {
		t.Fatal("expected protobuf decode error")
	}
	if called {
		t.Fatal("handler ran after protobuf decode failed")
	}

	err := router.Dispatch(&Context{Request: &Request{Route: 1, NeedReply: true}})
	if !errors.Is(err, handlerErr) {
		t.Fatalf("dispatch error=%v", err)
	}
	if !called {
		t.Fatal("handler did not run")
	}
}
