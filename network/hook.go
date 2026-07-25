package network

// Hooks 是 network 对业务层提供的全部生命周期钩子。
type Hooks struct {
	OnSessionStart func(*Session)
	OnSessionEnd   func(*Session)
	OnSessionBind  func(*Session)
	OnHeartbeat    func(*Session)
	OnReq          func(*ReqContext)
}
