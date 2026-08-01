package network

type Hooks struct {
	OnSessionStart func(*Session)
	OnSessionEnd   func(*Session)
	OnSessionBind  func(*Session) error
	OnHeartbeat    func(*Session)
	OnReq          func(*ReqContext)
}
