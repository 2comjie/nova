package network

import "context"

type Hooks struct {
	OnSessionStart func(*Session)
	OnSessionEnd   func(context.Context, *Session)
	OnSessionBind  func(*Session) error
	OnHeartbeat    func(*Session)
	OnReq          func(*ReqContext)
}
