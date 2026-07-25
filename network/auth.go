package network

// Auther 由业务层实现，用 token 解出 uid。
type Auther interface {
	Auth(token []byte) (uid string, err error)
}

// AuthFunc 让普通函数可以直接作为 Auther 使用。
type AuthFunc func(token []byte) (uid string, err error)

func (f AuthFunc) Auth(token []byte) (string, error) {
	return f(token)
}
