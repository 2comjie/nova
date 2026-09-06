package rpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestErrorRoundTrip(t *testing.T) {
	want := NewErrorWithDetail(1001, "not enough coins", []byte("details"))
	if FromError(want) != want {
		t.Fatal("business error was replaced")
	}
	body, err := proto.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got := new(Error)
	if err := proto.Unmarshal(body, got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(want, got) || got.Error() != want.Message {
		t.Fatalf("decoded error=%v", got)
	}
}

func TestFromError(t *testing.T) {
	for _, test := range []struct {
		err  error
		code uint32
	}{
		{context.Canceled, CodeCanceled},
		{context.DeadlineExceeded, CodeDeadlineExceeded},
		{errors.New("failed"), CodeInternal},
	} {
		got := FromError(test.err)
		if got.Code != test.code || got.Message != test.err.Error() {
			t.Fatalf("error=%v", got)
		}
	}
	if FromError(nil) != nil {
		t.Fatal("nil error changed")
	}
}
